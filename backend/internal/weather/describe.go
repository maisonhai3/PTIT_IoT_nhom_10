package weather

// wmoDescriptions maps WMO weather interpretation codes (as returned by Open-Meteo) to Vietnamese text.
var wmoDescriptions = map[int]string{
	0:  "Trời quang",
	1:  "Ít mây",
	2:  "Mây rải rác",
	3:  "Nhiều mây",
	45: "Sương mù",
	48: "Sương mù đóng băng",
	51: "Mưa phùn nhẹ",
	53: "Mưa phùn vừa",
	55: "Mưa phùn dày",
	56: "Mưa phùn lạnh nhẹ",
	57: "Mưa phùn lạnh dày",
	61: "Mưa nhẹ",
	63: "Mưa vừa",
	65: "Mưa to",
	66: "Mưa lạnh nhẹ",
	67: "Mưa lạnh to",
	71: "Tuyết nhẹ",
	73: "Tuyết vừa",
	75: "Tuyết dày",
	77: "Hạt tuyết",
	80: "Mưa rào nhẹ",
	81: "Mưa rào vừa",
	82: "Mưa rào dữ dội",
	85: "Mưa tuyết nhẹ",
	86: "Mưa tuyết nặng",
	95: "Dông",
	96: "Dông kèm mưa đá nhẹ",
	99: "Dông kèm mưa đá lớn",
}

// Describe returns the Vietnamese description of a WMO code.
func Describe(code int) string {
	if d, ok := wmoDescriptions[code]; ok {
		return d
	}
	return "Không rõ"
}

// IsWetCode reports whether the WMO code means falling water (drizzle, rain, snow, showers, thunderstorm).
func IsWetCode(code int) bool {
	switch {
	case code >= 51 && code <= 67:
		return true
	case code >= 71 && code <= 77:
		return true
	case code >= 80 && code <= 86:
		return true
	case code >= 95 && code <= 99:
		return true
	}
	return false
}
