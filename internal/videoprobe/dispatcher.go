// Package videoprobe 文档说明见 probe.go；本文件提供分派器签名自检
// （编译期常量）。功能分派已并入 probe.go 的 Probe 函数。
package videoprobe

// 编译期声明的容器白名单（供外部断言）。
var knownContainers = []string{"mp4", "webm"}

// IsKnownContainer 返回 c 是否在 VIDEO-01 E 档容器白名单内。
func IsKnownContainer(c string) bool {
	for _, k := range knownContainers {
		if k == c {
			return true
		}
	}
	return false
}
