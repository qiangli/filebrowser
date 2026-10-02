package users

import (
	"errors"
	"sync"
	"testing"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

func TestCommonPasswordValidationConcurrentFirstUse(t *testing.T) {
	const workers = 32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !isCommonPassword("password") {
				t.Error("known common password was accepted")
			}
			if isCommonPassword("not-in-the-list-unique-passphrase-2026") {
				t.Error("unique passphrase was rejected")
			}
		}()
	}
	wg.Wait()

	if _, err := ValidateAndHashPwd("password", 8); !errors.Is(err, fberrors.ErrEasyPassword) {
		t.Fatalf("ValidateAndHashPwd common password: got %v", err)
	}
	if _, err := ValidateAndHashPwd("password", 9); err != (fberrors.ErrShortPassword{MinimumLength: 9}) {
		t.Fatalf("minimum length check: got %v", err)
	}
}
