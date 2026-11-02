package notify

import (
	"strings"
	"testing"
)

func TestWindowsToastXMLIncludesIcon(t *testing.T) {
	xml := windowsToastXML(`A & B`, `body`, `file:///C:/icon.png`)
	if !strings.Contains(xml, `placement="appLogoOverride"`) {
		t.Fatalf("missing logo: %s", xml)
	}
	if !strings.Contains(xml, `src="file:///C:/icon.png"`) {
		t.Fatalf("missing src: %s", xml)
	}
	if !strings.Contains(xml, `A &amp; B`) {
		t.Fatalf("title not escaped: %s", xml)
	}
}

func TestWindowsToastXMLOmitsEmptyIcon(t *testing.T) {
	xml := windowsToastXML("title", "body", "")
	if strings.Contains(xml, "<image") {
		t.Fatalf("unexpected image: %s", xml)
	}
}
