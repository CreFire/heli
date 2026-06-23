package base

import "game/src/configdoc"

type ExcelConf = configdoc.DocPbConfig
type ExcelConfExt = configdoc.DocExtendConfig

type IGamerModBase interface {
	ModIndex() int
	OnGamerLogin(gamerId int64) error
	OnGamerLogout(gamerId int64) error
	OnDocReload(doc *ExcelConf, docExt *ExcelConfExt)
}

type GamerModBase struct {
	modIndex int
	doc      *ExcelConf
	docExt   *ExcelConfExt
}

func NewGamerModBase(modIndex int, doc *ExcelConf, docExt *ExcelConfExt) *GamerModBase {
	return &GamerModBase{modIndex: modIndex, doc: doc, docExt: docExt}
}

func (m *GamerModBase) ModIndex() int {
	return m.modIndex
}

func (m *GamerModBase) OnGamerLogin(gamerId int64) error {
	return nil
}

func (m *GamerModBase) OnGamerLogout(gamerId int64) error {
	return nil
}

func (m *GamerModBase) Doc() *ExcelConf {
	return m.doc
}

func (m *GamerModBase) DocExt() *ExcelConfExt {
	return m.docExt
}

func (m *GamerModBase) OnDocReload(doc *ExcelConf, docExt *ExcelConfExt) {
	m.docExt = docExt
	m.doc = doc
}
