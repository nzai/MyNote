package platform

import (
	"testing"

	"github.com/nzai/MyNote/internal/core"
)

func TestSaveDialogFilterMapping(t *testing.T) {
	textFilter, textExt, err := saveDialogFilter(core.KindText)
	if err != nil {
		t.Fatal(err)
	}
	if textFilter.Spec != "*.txt" || textExt != "txt" {
		t.Fatalf("text filter=%#v ext=%q", textFilter, textExt)
	}

	imageFilter, imageExt, err := saveDialogFilter(core.KindImage)
	if err != nil {
		t.Fatal(err)
	}
	if imageFilter.Spec != "*.png" || imageExt != "png" {
		t.Fatalf("image filter=%#v ext=%q", imageFilter, imageExt)
	}

	if _, _, err := saveDialogFilter(core.Kind(99)); err == nil {
		t.Fatal("unsupported kind must fail")
	}
}
