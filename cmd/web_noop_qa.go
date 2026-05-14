//go:build !qa

package cmd

import "github.com/sunerpy/pt-tools/web"

func wireQATestHooks(_ *web.Server, _ *chatopsBootstrap) {}
