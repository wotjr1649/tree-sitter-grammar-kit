// Package drivers embeds the native driver sources that the kit builds as separate
// executables with an external compiler. It contains no cgo: the C files are data.
package drivers

import "embed"

// NativeC holds src/drivers/native-c: the generic tsgk-native/r1 driver, its pinned
// index-euc-kr table and the pinned runtime source manifest.
//
//go:embed native-c/driver.c native-c/cp949_table.h native-c/runtime-manifest.json
var NativeC embed.FS
