package setups

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestModerationTemplates(t *testing.T) {
	h, err := NewModerationHandler(nil)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := time.Date(2026, 10, 28, 12, 0, 0, 123456000, time.UTC)
	setup := Setup{ID: "setup", Title: "<script>unsafe</script>", Visibility: Public, ModerationStatus: Pending, UpdatedAt: timestamp}
	data := reviewPage{Setup: &setup, TechnicalData: `{"motor":"<script>"}`, Reviews: []ReviewItem{{ID: setup.ID, Title: setup.Title, OwnerNickname: "<script>owner</script>", UpdatedAt: timestamp}}}
	for _, page := range []string{"moderation.html", "review.html"} {
		w := httptest.NewRecorder()
		h.render(w, page, data)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(w.Code, w.Body)
		}
		body := w.Body.String()
		for _, want := range []string{"/admin/moderation/setup/approve", "/admin/moderation/setup/reject", timestamp.Format(time.RFC3339Nano), "&lt;script&gt;", "<style>", "Admin navigation"} {
			if !strings.Contains(body, want) {
				t.Fatal(page, "missing", want)
			}
		}
		if strings.Contains(body, "<script>unsafe") || strings.Contains(body, "<script>owner") {
			t.Fatal("unescaped content", page)
		}
	}
}
