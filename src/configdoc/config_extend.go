package configdoc

type DocExtendConfig struct {
	pbDoc *DocPbConfig
}

var extenders = newExtendCheckHandlers()

var checkers = newExtendCheckHandlers()

type ConfigExtender interface {
	Extend(*DocPbConfig, *DocExtendConfig) bool
}

func newConfigExtend(pbConf *DocPbConfig) *DocExtendConfig {
	docExtend := &DocExtendConfig{pbDoc: pbConf}
	state := true
	for _, extendFunc := range extenders.handlers {
		if !extendFunc(pbConf, docExtend) {
			state = false
		}
	}
	if !state {
		return nil
	}
	return docExtend
}
