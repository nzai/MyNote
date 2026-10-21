//go:build windows

package platform

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSingleInstanceDecision(t *testing.T) {
	if got := decideInstance(true); got != InstanceSecondary {
		t.Fatalf("decideInstance(true) = %v, want secondary", got)
	}
	if got := decideInstance(false); got != InstanceOwner {
		t.Fatalf("decideInstance(false) = %v, want owner", got)
	}
}

func TestAcquireSingleInstanceOwnsThenRejectsSecond(t *testing.T) {
	name := fmt.Sprintf(`Local\ClipNote.Test.%d.%d`, os.Getpid(), time.Now().UnixNano())
	first, decision, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatal(err)
	}
	if decision != InstanceOwner {
		t.Fatalf("first decision = %v, want owner", decision)
	}

	second, decision, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatal(err)
	}
	if decision != InstanceSecondary || second != nil {
		t.Fatalf("second instance = %v, decision = %v", second, decision)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, decision, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatal(err)
	}
	if decision != InstanceOwner {
		t.Fatalf("third decision = %v, want owner", decision)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}
