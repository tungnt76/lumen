package auth

import (
	"testing"
	"time"
)

func TestTOTPRFC6238(t *testing.T) {
	// RFC 6238 test secret "12345678901234567890"; 8-digit vectors truncated to 6.
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	cases := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for ts, want := range cases {
		if !ValidTOTP(secret, want, time.Unix(ts, 0)) {
			t.Errorf("t=%d: code %s rejected", ts, want)
		}
	}
	if ValidTOTP(secret, "000000", time.Unix(59, 0)) {
		t.Error("wrong code accepted")
	}
	if !ValidTOTP(secret, "287082", time.Unix(59+30, 0)) {
		t.Error("one step of drift should be allowed")
	}
	if ValidTOTP(secret, "287082", time.Unix(59+120, 0)) {
		t.Error("old code accepted")
	}
}

func TestSessions(t *testing.T) {
	s := NewSessions([]byte("0123456789abcdef0123456789abcdef"))
	tok := s.Issue(42, time.Hour)
	if id, ok := s.Verify(tok); !ok || id != 42 {
		t.Fatalf("valid token rejected: %v %v", id, ok)
	}
	if _, ok := s.Verify(tok + "x"); ok {
		t.Error("tampered token accepted")
	}
	if _, ok := s.Verify("43" + tok[2:]); ok {
		t.Error("forged id accepted")
	}
	if _, ok := s.Verify(s.Issue(1, -time.Minute)); ok {
		t.Error("expired token accepted")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allowed("ip") {
			t.Fatal("locked too early")
		}
		l.Fail("ip")
	}
	if l.Allowed("ip") {
		t.Error("should be locked after 3 failures")
	}
	l.Reset("ip")
	if !l.Allowed("ip") {
		t.Error("reset should unlock")
	}
}
