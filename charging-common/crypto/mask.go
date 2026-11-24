package crypto

// MaskPhone keeps the first 3 and last 4 digits: 138****8888.
func MaskPhone(s string) string {
	if len(s) < 7 {
		return "****"
	}
	return s[:3] + "****" + s[len(s)-4:]
}

// MaskIDCard keeps first 4 and last 2 characters.
func MaskIDCard(s string) string {
	if len(s) < 7 {
		return "****"
	}
	n := len(s) - 6
	out := make([]byte, 0, len(s))
	out = append(out, s[:4]...)
	for i := 0; i < n; i++ {
		out = append(out, '*')
	}
	return string(append(out, s[len(s)-2:]...))
}

// MaskVIN keeps first 3 and last 3 characters.
func MaskVIN(s string) string {
	if len(s) < 7 {
		return "****"
	}
	return s[:3] + "******" + s[len(s)-3:]
}
