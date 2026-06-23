package configdoc

import (
	"strconv"
	"strings"

	"game/deps/misc"
	"game/deps/xlog"
)

func checkExcelVersion(doc *DocPbConfig) {
	if doc == nil {
		return
	}
	if doc.ExcelVersion == 0 {
		xlog.Warnf("doc excel version missing")
	}
	buildVer := strings.TrimSpace(misc.ExcelVer)
	if buildVer == "" {
		return
	}
	val, err := strconv.ParseInt(buildVer, 10, 32)
	if err != nil {
		xlog.Warnf("invalid build excel version: %v", err)
		return
	}
	if int32(val) != doc.ExcelVersion {
		xlog.Infof("excel version mismatch: build=%d doc=%d", val, doc.ExcelVersion)
	}
}
