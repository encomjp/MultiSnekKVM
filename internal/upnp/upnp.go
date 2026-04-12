package upnp

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"multisnekkvm/internal/logutil"
)

// Manager handles UPnP-IGD port forwarding for NAT traversal.
type Manager struct {
	controlURL  string
	serviceType string
	localIP     string
	port        int
	mapped      bool
}

func NewManager(port int) *Manager {
	return &Manager{port: port}
}

// Setup discovers the gateway and adds a port mapping.
func (u *Manager) Setup(ctx context.Context) {
	localIP := getOutboundIP()
	if localIP == "" {
		log.Println("upnp: cannot determine local IP")
		return
	}
	u.localIP = localIP

	controlURL, serviceType, err := discoverGateway(ctx)
	if err != nil {
		log.Printf("upnp: gateway discovery failed: %v", err)
		return
	}
	u.controlURL = controlURL
	u.serviceType = serviceType
	log.Printf("upnp: found gateway at %s", controlURL)

	if err := u.addMapping(); err != nil {
		log.Printf("upnp: port mapping failed: %v", err)
		return
	}
	u.mapped = true
	log.Printf("upnp: mapped external port %d → %s:%d", u.port, u.localIP, u.port)

	logutil.SafeGo("upnp-refresh", func() {
		ticker := time.NewTicker(20 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				u.removeMapping()
				return
			case <-ticker.C:
				if err := u.addMapping(); err != nil {
					log.Printf("upnp: mapping refresh failed: %v", err)
				}
			}
		}
	})
}

func (u *Manager) IsMapped() bool {
	return u.mapped
}

func (u *Manager) addMapping() error {
	return soapAction(u.controlURL, u.serviceType, "AddPortMapping", fmt.Sprintf(
		`<NewRemoteHost></NewRemoteHost>
		<NewExternalPort>%d</NewExternalPort>
		<NewProtocol>TCP</NewProtocol>
		<NewInternalPort>%d</NewInternalPort>
		<NewInternalClient>%s</NewInternalClient>
		<NewEnabled>1</NewEnabled>
		<NewPortMappingDescription>Multisnek</NewPortMappingDescription>
		<NewLeaseDuration>3600</NewLeaseDuration>`,
		u.port, u.port, u.localIP))
}

func (u *Manager) removeMapping() {
	if !u.mapped || u.controlURL == "" {
		return
	}
	err := soapAction(u.controlURL, u.serviceType, "DeletePortMapping", fmt.Sprintf(
		`<NewRemoteHost></NewRemoteHost>
		<NewExternalPort>%d</NewExternalPort>
		<NewProtocol>TCP</NewProtocol>`,
		u.port))
	if err != nil {
		log.Printf("upnp: remove mapping failed: %v", err)
	} else {
		log.Printf("upnp: removed port mapping for %d", u.port)
	}
	u.mapped = false
}

func (u *Manager) GetExternalIP() string {
	if u.controlURL == "" {
		return ""
	}
	body, err := soapRequest(u.controlURL, u.serviceType, "GetExternalIPAddress", "")
	if err != nil {
		return ""
	}
	start := strings.Index(body, "<NewExternalIPAddress>")
	end := strings.Index(body, "</NewExternalIPAddress>")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return body[start+len("<NewExternalIPAddress>") : end]
}

func discoverGateway(ctx context.Context) (controlURL, serviceType string, err error) {
	serviceTypes := []string{
		"urn:schemas-upnp-org:service:WANIPConnection:1",
		"urn:schemas-upnp-org:service:WANIPConnection:2",
		"urn:schemas-upnp-org:service:WANPPPConnection:1",
	}

	for _, st := range serviceTypes {
		url, err := ssdpSearch(ctx, st)
		if err != nil {
			continue
		}
		controlURL, err := getControlURL(url, st)
		if err != nil {
			continue
		}
		return controlURL, st, nil
	}

	return "", "", fmt.Errorf("no UPnP gateway found")
}

func ssdpSearch(ctx context.Context, serviceType string) (string, error) {
	searchMsg := fmt.Sprintf(
		"M-SEARCH * HTTP/1.1\r\n"+
			"HOST: 239.255.255.250:1900\r\n"+
			"MAN: \"ssdp:discover\"\r\n"+
			"MX: 3\r\n"+
			"ST: %s\r\n"+
			"\r\n", serviceType)

	addr, err := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		return "", err
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(4 * time.Second)
	}
	conn.SetDeadline(deadline)

	if _, err := conn.WriteToUDP([]byte(searchMsg), addr); err != nil {
		return "", err
	}

	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return "", fmt.Errorf("ssdp timeout for %s", serviceType)
		}

		response := string(buf[:n])
		if !strings.Contains(response, serviceType) {
			continue
		}

		for _, line := range strings.Split(response, "\r\n") {
			lower := strings.ToLower(line)
			if strings.HasPrefix(lower, "location:") {
				return strings.TrimSpace(line[len("location:"):]), nil
			}
		}
	}
}

type upnpRoot struct {
	Device upnpDevice `xml:"device"`
}

type upnpDevice struct {
	DeviceType string        `xml:"deviceType"`
	Devices    []upnpDevice  `xml:"deviceList>device"`
	Services   []upnpService `xml:"serviceList>service"`
}

type upnpService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

func getControlURL(descURL, serviceType string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(descURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var root upnpRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return "", err
	}

	controlURL := findServiceURL(&root.Device, serviceType)
	if controlURL == "" {
		return "", fmt.Errorf("service %s not found in device description", serviceType)
	}

	if strings.HasPrefix(controlURL, "/") {
		base := descURL
		if idx := strings.Index(base, "//"); idx >= 0 {
			if slash := strings.Index(base[idx+2:], "/"); slash >= 0 {
				base = base[:idx+2+slash]
			}
		}
		controlURL = base + controlURL
	}

	return controlURL, nil
}

func findServiceURL(device *upnpDevice, serviceType string) string {
	for _, svc := range device.Services {
		if svc.ServiceType == serviceType {
			return svc.ControlURL
		}
	}
	for _, sub := range device.Devices {
		if url := findServiceURL(&sub, serviceType); url != "" {
			return url
		}
	}
	return ""
}

func soapAction(controlURL, serviceType, action, args string) error {
	_, err := soapRequest(controlURL, serviceType, action, args)
	return err
}

func soapRequest(controlURL, serviceType, action, args string) (string, error) {
	soapBody := fmt.Sprintf(
		`<?xml version="1.0"?>
		<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
			s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
			<s:Body>
				<u:%s xmlns:u="%s">%s</u:%s>
			</s:Body>
		</s:Envelope>`, action, serviceType, args, action)

	req, err := http.NewRequest("POST", controlURL, strings.NewReader(soapBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", fmt.Sprintf("\"%s#%s\"", serviceType, action))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return string(body), fmt.Errorf("SOAP %s returned %d", action, resp.StatusCode)
	}

	return string(body), nil
}

func getOutboundIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
