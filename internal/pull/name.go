package pull

import (
	"strings"
	"unicode/utf8"
)

// maxNameBytes keeps names well under every file system's limit (255).
const maxNameBytes = 200

// CleanName turns an untrusted file name into one safe on every
// platform: one path element (the last after "/"), no separators,
// control or Windows-reserved characters, no reserved device names, no
// trailing dot or space, at most maxNameBytes, never empty, "." or "..".
// It keeps the extension when it shortens a name.
func CleanName(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToValidUTF8(name, "_")
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	name = truncate(name, maxNameBytes)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "file"
	}
	if reserved(name) {
		name = truncate("_"+name, maxNameBytes)
		name = strings.TrimRight(name, ". ")
	}
	return name
}

// reserved reports whether name is a Windows device name (CON, NUL.txt...).
func reserved(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	switch strings.ToUpper(strings.TrimRight(stem, " ")) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}

// truncate shortens name to max bytes on a rune boundary, keeping a
// short extension.
func truncate(name string, max int) string {
	if len(name) <= max {
		return name
	}
	ext := ""
	if i := strings.LastIndex(name, "."); i > 0 && len(name)-i <= 16 {
		ext = name[i:]
	}
	stem := name[:len(name)-len(ext)]
	limit := max - len(ext)
	for limit > 0 && !utf8.RuneStart(stem[limit]) {
		limit--
	}
	return stem[:limit] + ext
}
