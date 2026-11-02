package notify

import "strings"

func windowsToastXML(title, body, iconSrc string) string {
	var b strings.Builder
	b.WriteString(`<toast><visual><binding template="ToastGeneric">`)
	if iconSrc != "" {
		b.WriteString(`<image placement="appLogoOverride" src="`)
		b.WriteString(xmlEscape(iconSrc))
		b.WriteString(`"/>`)
	}
	b.WriteString(`<text>`)
	b.WriteString(xmlEscape(title))
	b.WriteString(`</text><text>`)
	b.WriteString(xmlEscape(body))
	b.WriteString(`</text></binding></visual><audio src="ms-winsoundevent:Notification.Default"/></toast>`)
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	).Replace(s)
}
