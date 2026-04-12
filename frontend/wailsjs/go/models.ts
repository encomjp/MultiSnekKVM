export namespace app {
	
	export class PeerInfo {
	    id: string;
	    name: string;
	    address: string;
	    addresses: string[];
	    fingerprint: string;
	    source: string;
	    routes: string[];
	    preferredRoute: string;
	    trusted: boolean;
	    status: string;
	    lastSeen: number;
	
	    static createFrom(source: any = {}) {
	        return new PeerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.address = source["address"];
	        this.addresses = source["addresses"];
	        this.fingerprint = source["fingerprint"];
	        this.source = source["source"];
	        this.routes = source["routes"];
	        this.preferredRoute = source["preferredRoute"];
	        this.trusted = source["trusted"];
	        this.status = source["status"];
	        this.lastSeen = source["lastSeen"];
	    }
	}
	export class SaveReceivedFilesResult {
	    saved: string[];
	    dest: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveReceivedFilesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.saved = source["saved"];
	        this.dest = source["dest"];
	    }
	}
	export class SessionStatus {
	    connected: boolean;
	    controlling: boolean;
	    peerName: string;
	    peerID: string;
	    role: string;
	    latencyMs: number;
	    audioLatencyMs: number;
	    jitterMs: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.controlling = source["controlling"];
	        this.peerName = source["peerName"];
	        this.peerID = source["peerID"];
	        this.role = source["role"];
	        this.latencyMs = source["latencyMs"];
	        this.audioLatencyMs = source["audioLatencyMs"];
	        this.jitterMs = source["jitterMs"];
	    }
	}

}

export namespace audio {
	
	export class AudioDevice {
	    id: string;
	    name: string;
	    flow: string;
	
	    static createFrom(source: any = {}) {
	        return new AudioDevice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.flow = source["flow"];
	    }
	}

}

export namespace identity {
	
	export class DeviceInfo {
	    id: string;
	    name: string;
	    fingerprint: string;
	    pairingCode?: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.fingerprint = source["fingerprint"];
	        this.pairingCode = source["pairingCode"];
	        this.port = source["port"];
	    }
	}

}

export namespace input {
	
	export class MonitorInfo {
	    id: string;
	    name: string;
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	    isPrimary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MonitorInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.isPrimary = source["isPrimary"];
	    }
	}

}

export namespace logutil {
	
	export class LogEvent {
	    level: string;
	    pattern: string;
	    count: number;
	    sample: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.level = source["level"];
	        this.pattern = source["pattern"];
	        this.count = source["count"];
	        this.sample = source["sample"];
	    }
	}
	export class LogAnalysis {
	    events: LogEvent[];
	    totalErrors: number;
	    windowLines: number;
	    analyzedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new LogAnalysis(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.events = this.convertValues(source["events"], LogEvent);
	        this.totalErrors = source["totalErrors"];
	        this.windowLines = source["windowLines"];
	        this.analyzedAt = source["analyzedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace resilience {
	
	export class SubsystemStatus {
	    name: string;
	    healthy: boolean;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new SubsystemStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.healthy = source["healthy"];
	        this.detail = source["detail"];
	    }
	}
	export class HealthStatus {
	    healthy: boolean;
	    reconnecting: boolean;
	    subsystems: SubsystemStatus[];
	    uptime: number;
	    goroutines: number;
	    goroutineDelta: number;
	
	    static createFrom(source: any = {}) {
	        return new HealthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.healthy = source["healthy"];
	        this.reconnecting = source["reconnecting"];
	        this.subsystems = this.convertValues(source["subsystems"], SubsystemStatus);
	        this.uptime = source["uptime"];
	        this.goroutines = source["goroutines"];
	        this.goroutineDelta = source["goroutineDelta"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace tailscale {
	
	export class Status {
	    available: boolean;
	    connected: boolean;
	    backendState: string;
	    selfName: string;
	    tailnet: string;
	    selfIPs: string[];
	    peerCount: number;
	    targetCount: number;
	    lastSync: number;
	    lastError: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.connected = source["connected"];
	        this.backendState = source["backendState"];
	        this.selfName = source["selfName"];
	        this.tailnet = source["tailnet"];
	        this.selfIPs = source["selfIPs"];
	        this.peerCount = source["peerCount"];
	        this.targetCount = source["targetCount"];
	        this.lastSync = source["lastSync"];
	        this.lastError = source["lastError"];
	    }
	}

}

