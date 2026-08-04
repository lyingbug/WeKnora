package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestChunkFeedbackUserIDMatchesSessionPrincipalWidth(t *testing.T) {
	feedbackField, ok := reflect.TypeOf(ChunkFeedback{}).FieldByName("UserID")
	if !ok {
		t.Fatal("ChunkFeedback.UserID field not found")
	}
	sessionField, ok := reflect.TypeOf(Session{}).FieldByName("UserID")
	if !ok {
		t.Fatal("Session.UserID field not found")
	}

	if got := feedbackField.Tag.Get("gorm"); !strings.Contains(got, "type:varchar(512)") {
		t.Fatalf("ChunkFeedback.UserID gorm tag = %q, want varchar(512)", got)
	}
	if got := sessionField.Tag.Get("gorm"); !strings.Contains(got, "type:varchar(512)") {
		t.Fatalf("Session.UserID gorm tag = %q, want varchar(512)", got)
	}
}

func TestNormalizeDislikeReason(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  DislikeReasonType
		ok    bool
	}{
		{name: "reason code", input: "inaccurate", want: DislikeReasonInaccurate, ok: true},
		{name: "padded and upper case code", input: "  Unclear ", want: DislikeReasonUnclear, ok: true},
		{name: "legacy chinese label", input: "与问题不相关", want: DislikeReasonIrrelevant, ok: true},
		{name: "free-form text", input: "这段引用完全对不上我的问题", ok: false},
		{name: "empty", input: "   ", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeDislikeReason(tc.input)
			if ok != tc.ok {
				t.Fatalf("NormalizeDislikeReason(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("NormalizeDislikeReason(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetDislikeReasonsExposesCodes(t *testing.T) {
	options := GetDislikeReasons()
	if len(options) != len(AllDislikeReasons()) {
		t.Fatalf("GetDislikeReasons() returned %d options, want %d", len(options), len(AllDislikeReasons()))
	}
	for i, reason := range AllDislikeReasons() {
		if options[i].Code != reason {
			t.Fatalf("option %d code = %q, want %q", i, options[i].Code, reason)
		}
		if options[i].Label == "" {
			t.Fatalf("option %d (%q) has an empty label", i, reason)
		}
	}
}
