package platform

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
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

func TestSaveDialogCanceledHresultIsRecognized(t *testing.T) {
	canceled := co.ERROR_CANCELLED.ToHresult()
	for _, err := range []error{
		canceled,
		fmt.Errorf("show Win32 save dialog: %w", canceled),
		co.ERROR_CANCELLED,
	} {
		if !isSaveDialogCanceled(err) {
			t.Fatalf("error %v must be treated as save-dialog cancellation", err)
		}
	}

	if isSaveDialogCanceled(errors.New("permission denied")) {
		t.Fatal("ordinary save error must not be treated as cancellation")
	}
}
