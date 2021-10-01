package webview

import (
	"context"
	"fmt"
	"os"
)

type FileViewManager struct {
	chartFile *os.File
	Smgr      *StatsMgr
	Ctx       context.Context
	Cancel    context.CancelFunc
	Views     []Viewer
}

func NewFileViewManager(filename string) (*FileViewManager, error) {
	vm := FileViewManager{}
	vm.chartFile = os.Stdout
	if filename != "-" {
		var err error
		vm.chartFile, err = os.Create(filename)
		if err != nil {
			return nil, fmt.Errorf("ViewManager can not open file %s: %s", filename, err)
		}
	}
	return &vm, nil
}

func (vm *FileViewManager) Start(ctx context.Context) {
	vm.Ctx = ctx

}

func (vm *FileViewManager) Sttop() {

}
