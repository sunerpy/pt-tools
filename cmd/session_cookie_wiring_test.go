package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type renewedCall struct {
	site    string
	chain   []string
	renewed string
}

type fakeRenewedStore struct {
	stored bool
	err    error
	calls  []renewedCall
}

func (f *fakeRenewedStore) SaveRenewedSiteCookie(site string, chain []string, renewed string) (bool, error) {
	f.calls = append(f.calls, renewedCall{site, chain, renewed})
	return f.stored, f.err
}

// 续期换发的 Cookie 连同这个会话用过的那些原样交给 ConfigStore，存没存上、出没出错都原样回给站点实例。
func TestRenewedCookieSink(t *testing.T) {
	store := &fakeRenewedStore{stored: true}
	sink := renewedCookieSink(store)
	stored, err := sink("byrpt", []string{"a=0", "a=1"}, "a=1")
	assert.True(t, stored)
	assert.NoError(t, err)
	assert.Equal(t, []renewedCall{{"byrpt", []string{"a=0", "a=1"}, "a=1"}}, store.calls)

	store.stored, store.err = false, errors.New("database is locked")
	stored, err = sink("byrpt", []string{"a=1", "a=2"}, "a=2")
	assert.False(t, stored)
	assert.EqualError(t, err, "database is locked")

	store.err = nil
	stored, err = sink("byrpt", []string{"x=1", "x=2"}, "x=2")
	assert.False(t, stored, "设置里存的不是这个会话的 Cookie 时不存")
	assert.NoError(t, err)
}
