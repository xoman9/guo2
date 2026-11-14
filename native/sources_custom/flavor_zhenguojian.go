//go:build zhenguojian

package custom

import (
 "duanjuapp/native/core"
 _ "duanjuapp/native/sources"
)

func init() {
 core.RegisterFlavor("zhenguojian", []string{
  "lujj31", "liuguo", "xiaoguo",
  "youguo", "meiguo", "yingguo", "luguo", "liguo",
  "juguo", "ningguo", "mangguo", "chengguodj", "jiaoguo",
  "laosebizy.com", "jable", "missav",
 })
}
