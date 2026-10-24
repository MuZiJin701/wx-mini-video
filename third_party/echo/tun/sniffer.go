package tun

// extractSNI parses a TLS ClientHello message and extracts the SNI hostname.
func extractSNI(data []byte) string {
	if len(data) < 5 || data[0] != 22 {
		return ""
	}
	recordLen := int(data[3])<<8 | int(data[4])
	data = data[5:]
	if len(data) > recordLen {
		data = data[:recordLen]
	}

	if len(data) < 4 || data[0] != 1 {
		return ""
	}
	data = data[4:]

	if len(data) < 34 {
		return ""
	}
	data = data[34:]

	if len(data) < 1 {
		return ""
	}
	n := int(data[0])
	data = data[1:]
	if len(data) < n {
		return ""
	}
	data = data[n:]

	if len(data) < 2 {
		return ""
	}
	n = int(data[0])<<8 | int(data[1])
	data = data[2:]
	if len(data) < n {
		return ""
	}
	data = data[n:]

	if len(data) < 1 {
		return ""
	}
	n = int(data[0])
	data = data[1:]
	if len(data) < n {
		return ""
	}
	data = data[n:]

	if len(data) < 2 {
		return ""
	}
	extLen := int(data[0])<<8 | int(data[1])
	data = data[2:]
	if len(data) > extLen {
		data = data[:extLen]
	}

	for len(data) >= 4 {
		extType := int(data[0])<<8 | int(data[1])
		eLen := int(data[2])<<8 | int(data[3])
		data = data[4:]
		if len(data) < eLen {
			return ""
		}
		if extType == 0 {
			d := data[:eLen]
			if len(d) < 5 {
				return ""
			}
			d = d[2:]
			if d[0] != 0 {
				return ""
			}
			nameLen := int(d[1])<<8 | int(d[2])
			d = d[3:]
			if len(d) >= nameLen {
				return string(d[:nameLen])
			}
			return ""
		}
		data = data[eLen:]
	}
	return ""
}
