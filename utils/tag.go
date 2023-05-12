package utils

import "strings"

// 把结构体后读取的tag，解析成数组
func ParseTag(tag string) []string {
	splits := strings.Split(tag, ",")
	for i := range splits {
		splits[i] = strings.Trim(splits[i], " ")
	}
	return splits
}