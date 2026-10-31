//go:build !windows

package i18n

func detectOSLocalePlatform() string {
	return "en"
}
