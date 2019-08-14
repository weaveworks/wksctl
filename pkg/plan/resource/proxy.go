package resource

import "fmt"

const unsetProxy = "unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY"

func withoutProxy(script string) string {
	return fmt.Sprintf("( %s && ( %s ) )", unsetProxy, script)
}
