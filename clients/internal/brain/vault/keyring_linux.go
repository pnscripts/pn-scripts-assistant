//go:build linux && cgo

package vault

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdlib.h>
#include <string.h>

// libsecret's own structures, as its headers define them. Declared here so the
// program needs libsecret only at run time — loaded if it is there, which on
// any desktop with GTK it is, and the key file used if it is not.
typedef struct { const char *name; int type; } pn_attr;
typedef struct {
	const char *name;
	int flags;
	pn_attr attributes[32];
	int reserved;
	void *reserved1, *reserved2, *reserved3, *reserved4, *reserved5, *reserved6, *reserved7;
} pn_schema;
typedef struct { unsigned int domain; int code; char *message; } pn_gerror;

typedef int (*pn_store_fn)(const pn_schema *, const char *, const char *, const char *, void *, pn_gerror **, ...);
typedef char *(*pn_lookup_fn)(const pn_schema *, void *, pn_gerror **, ...);
typedef int (*pn_clear_fn)(const pn_schema *, void *, pn_gerror **, ...);
typedef void (*pn_free_fn)(void *);

static pn_store_fn pn_store_p;
static pn_lookup_fn pn_lookup_p;
static pn_clear_fn pn_clear_p;
static pn_free_fn pn_secret_free_p;
static pn_free_fn pn_error_free_p;

static pn_schema pn_the_schema = {
	"org.pnscripts.Assistant.BrainKey", 0,
	{ { "brain-key", 0 }, { NULL, 0 } },
};

static int pn_load(void) {
	void *lib = dlopen("libsecret-1.so.0", RTLD_NOW | RTLD_GLOBAL);
	if (!lib) return 0;
	pn_store_p = (pn_store_fn) dlsym(lib, "secret_password_store_sync");
	pn_lookup_p = (pn_lookup_fn) dlsym(lib, "secret_password_lookup_sync");
	pn_clear_p = (pn_clear_fn) dlsym(lib, "secret_password_clear_sync");
	pn_secret_free_p = (pn_free_fn) dlsym(lib, "secret_password_free");
	pn_error_free_p = (pn_free_fn) dlsym(RTLD_DEFAULT, "g_error_free");
	return pn_store_p && pn_lookup_p && pn_clear_p && pn_secret_free_p;
}

static char *pn_take_error(pn_gerror *e) {
	if (!e) return NULL;
	char *msg = e->message ? strdup(e->message) : strdup("unknown error");
	if (pn_error_free_p) pn_error_free_p(e);
	return msg;
}

static int pn_store(const char *id, const char *label, const char *secret, char **err) {
	pn_gerror *e = NULL;
	int ok = pn_store_p(&pn_the_schema, "default", label, secret, NULL, &e, "brain-key", id, NULL);
	*err = pn_take_error(e);
	return ok;
}

static char *pn_lookup(const char *id, char **err) {
	pn_gerror *e = NULL;
	char *found = pn_lookup_p(&pn_the_schema, NULL, &e, "brain-key", id, NULL);
	*err = pn_take_error(e);
	if (!found) return NULL;
	char *copy = strdup(found);
	pn_secret_free_p(found);
	return copy;
}

static int pn_clear(const char *id, char **err) {
	pn_gerror *e = NULL;
	int ok = pn_clear_p(&pn_the_schema, NULL, &e, "brain-key", id, NULL);
	*err = pn_take_error(e);
	return ok;
}
*/
import "C"

import (
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"
)

/*
 * The Secret Service — GNOME Keyring, KWallet — through libsecret, loaded at
 * run time. Only offered when there is a session bus to reach it on; asked
 * with a time limit, because a keyring waiting on a password prompt nobody
 * sees must not hold the program's start for ever.
 */
type secretService struct{}

var loadSecret = sync.OnceValue(func() bool { return C.pn_load() != 0 })

func platformKeyring() (Keyring, bool) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" && os.Getenv("XDG_RUNTIME_DIR") == "" {
		return nil, false
	}

	if !loadSecret() {
		return nil, false
	}

	return secretService{}, true
}

func (secretService) Name() string    { return "this computer's keyring (Secret Service)" }
func (secretService) Warning() string { return "" }

// HowLongTheKeyringMayTake bounds one question to it.
const HowLongTheKeyringMayTake = 20 * time.Second

func withTime[T any](do func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}

	done := make(chan result, 1)

	go func() {
		v, err := do()
		done <- result{v, err}
	}()

	select {
	case r := <-done:
		return r.v, r.err
	case <-time.After(HowLongTheKeyringMayTake):
		var zero T

		return zero, errors.New("the keyring did not answer")
	}
}

func cError(e *C.char) error {
	if e == nil {
		return nil
	}

	defer C.free(unsafe.Pointer(e))

	return errors.New(C.GoString(e))
}

func (secretService) Get(id string) ([]byte, error) {
	return withTime(func() ([]byte, error) {
		cid := C.CString(id)
		defer C.free(unsafe.Pointer(cid))

		var cerr *C.char

		found := C.pn_lookup(cid, &cerr)
		if err := cError(cerr); err != nil {
			return nil, err
		}

		if found == nil {
			return nil, ErrNotFound
		}

		defer C.free(unsafe.Pointer(found))

		return hex.DecodeString(strings.TrimSpace(C.GoString(found)))
	})
}

func (secretService) Set(id string, secret []byte) error {
	_, err := withTime(func() (bool, error) {
		cid, label, value := C.CString(id), C.CString("PN Scripts Assistant brain key "+id),
			C.CString(hex.EncodeToString(secret))
		defer C.free(unsafe.Pointer(cid))
		defer C.free(unsafe.Pointer(label))
		defer C.free(unsafe.Pointer(value))

		var cerr *C.char

		ok := C.pn_store(cid, label, value, &cerr)
		if err := cError(cerr); err != nil {
			return false, err
		}

		if ok == 0 {
			return false, errors.New("the keyring did not keep it")
		}

		return true, nil
	})

	return err
}

func (secretService) Delete(id string) error {
	_, err := withTime(func() (bool, error) {
		cid := C.CString(id)
		defer C.free(unsafe.Pointer(cid))

		var cerr *C.char

		C.pn_clear(cid, &cerr)

		return true, cError(cerr)
	})

	return err
}
