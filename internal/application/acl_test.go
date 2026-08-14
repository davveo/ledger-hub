package application

import (
	"testing"

	"github.com/davveo/ledger-hub/internal/domain"
)

func TestACLAllow(t *testing.T) {
	acl := NewACL([]domain.ACLRule{
		{SourceSystem: "campaign", Commands: []string{"Credit"}, Assets: []string{"POINT"}},
		{SourceSystem: "order", Commands: []string{"Freeze", "Capture", "Release"}, Assets: []string{"POINT"}},
		{SourceSystem: "pay", Commands: []string{"Credit"}, Assets: []string{"BALANCE_CNY"}},
		{SourceSystem: "worker", Commands: []string{"Release"}, Assets: []string{"*"}},
	})
	cases := []struct {
		src   string
		cmd   domain.Command
		asset string
		ok    bool
	}{
		{"campaign", domain.CmdCredit, "POINT", true},
		{"campaign", domain.CmdCredit, "BALANCE_CNY", false},
		{"order", domain.CmdCredit, "POINT", false},
		{"order", domain.CmdFreeze, "POINT", true},
		{"pay", domain.CmdCredit, "BALANCE_CNY", true},
		{"pay", domain.CmdDebit, "BALANCE_CNY", false},
		{"worker", domain.CmdRelease, "POINT", true},
	}
	for _, tc := range cases {
		got := acl.Allow(tc.src, tc.cmd, tc.asset)
		if got != tc.ok {
			t.Fatalf("%s %s %s: got %v want %v", tc.src, tc.cmd, tc.asset, got, tc.ok)
		}
	}
}

func TestACLEmptyAllowsAll(t *testing.T) {
	acl := NewACL(nil)
	if !acl.Allow("anyone", domain.CmdCredit, "POINT") {
		t.Fatal("empty ACL should allow")
	}
}
