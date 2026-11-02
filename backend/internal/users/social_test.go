package users

import (
	"strings"
	"testing"
)

func TestSocialURLValidation(t *testing.T) {
	for _, tc := range []struct{ platform, url, want string }{
		{"instagram", " https://WWW.INSTAGRAM.COM/driver/ ", "https://www.instagram.com/driver"},
		{"tiktok", "https://www.tiktok.com/@driver.name", "https://www.tiktok.com/@driver.name"},
		{"youtube", "https://youtube.com/@driver", "https://youtube.com/@driver"},
		{"youtube", "https://www.youtube.com/channel/UC123", "https://www.youtube.com/channel/UC123"},
		{"youtube", "https://youtube.com/user/driver", "https://youtube.com/user/driver"},
		{"youtube", "https://youtube.com/c/driver", "https://youtube.com/c/driver"},
		{"instagram", "", ""},
	} {
		t.Run(tc.url, func(t *testing.T) {
			got, err := ValidateSocialURL(tc.platform, tc.url)
			if err != nil || got != tc.want {
				t.Fatal(got, err)
			}
		})
	}
	for _, value := range []string{
		"javascript:alert(1)", "data:text/html,evil", "http://instagram.com/driver", "//instagram.com/driver",
		"https://instagram.com.evil.test/driver", "https://evil.test/instagram.com/driver", "https://instagram.com@evil.test/driver",
		"https://evil@instagram.com/driver", "https://instagram.com:443/driver", "https://instagram.com./driver",
		"https://instagram.com/driver?next=https://evil.test", "https://instagram.com/driver#evil", "https://instagram.com/driver?",
		"https://instagram.com/driver#", "https://instagram.com/l.php", "https://instagram.com/accounts/login",
		"https://instagram.com/redirect", "https://instagram.com/..", "https://instagram.com/%64river",
		"https://instagram.com/driver\\evil", "https://instagram.com/driver\x00", "https://instagram.com/driver\nBAD",
		"https://instagram.com/" + strings.Repeat("x", 2048), "https://instagram.com/driver/../accounts",
		"https://instagram.com/driver//", "https://instagram.com/", "https://instagram.com/driver%3fnext=evil",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := ValidateSocialURL("instagram", value); err == nil {
				t.Fatal("unsafe URL accepted", value)
			}
		})
	}
	for _, tc := range []struct{ platform, url string }{
		{"tiktok", "https://vm.tiktok.com/redirect"}, {"tiktok", "https://tiktok.com/redirect"},
		{"youtube", "https://youtu.be/abc"}, {"youtube", "https://youtube.com/redirect?q=evil"},
		{"youtube", "https://youtube.com/watch?v=abc"}, {"youtube", "https://youtube.com/@driver?next=evil"},
		{"instagram", "https://youtube.com/@driver"},
	} {
		if _, err := ValidateSocialURL(tc.platform, tc.url); err == nil {
			t.Fatal("unsafe/wrong platform URL accepted", tc)
		}
	}
}
