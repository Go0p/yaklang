package httptpl

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func MockEchoPlugin(onTokens ...func(string)) (string, error) {
	var name = utils.RandStringBytes(10)
	var raw = `
id: TEST_` + name + `
info:
  name: NAME_` + name + `
  author: v1ll4n

requests:
  - raw:
    - |
      GET /aaa-testcase-mock-echo-plugin/` + name + ` HTTP/1.1
      Host: {{Hostname}}
      
      abc
    matchers:
    - type: word
      words:
        - "` + name + `"
`
	defer func() {
		for _, handler := range onTokens {
			handler(name)
		}
	}()
	return yakit.CreateTemporaryYakScript("nuclei", raw)
}
