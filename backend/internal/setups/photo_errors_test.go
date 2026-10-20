package setups

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BlinovDev/rc-setup-hub/backend/internal/photos"
)

func TestPhotoFailureReasons(t *testing.T) {
	for _, tc := range []struct {
		err     error
		status  int
		message string
	}{
		{photos.ErrFormat, 400, "unsupported image format"},
		{photos.ErrDecode, 400, "image could not be decoded"},
		{photos.ErrSize, 400, "at most 25 MiB"},
		{photos.ErrDimensions, 400, "at most 50 megapixels"},
		{invalid("a setup may have at most 10 photos"), 400, "at most 10 photos"},
		{photos.ErrBusy, 503, "busy"},
		{errors.New("private decoder/filesystem detail"), 500, "setup operation failed"},
	} {
		w := httptest.NewRecorder()
		photoFailure(w, tc.err)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.message) || strings.Contains(w.Body.String(), "private decoder") {
			t.Fatal(w.Code, w.Body)
		}
	}
}
