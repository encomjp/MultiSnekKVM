package input

func NormalizeEdgeSide(side string) string {
	switch side {
	case "left", "right", "top", "bottom":
		return side
	default:
		return ""
	}
}
