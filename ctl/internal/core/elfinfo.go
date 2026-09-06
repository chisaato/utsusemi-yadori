package core

import (
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

var errNotELF = errors.New("not an ELF file")

func IsNotELF(err error) bool { return errors.Is(err, errNotELF) }

// archByGOARCH 将 Go GOARCH 映射为项目规范架构名（frida 资产命名同构）
var archByGOARCH = map[string]string{
	"arm64": "arm64",
	"arm":   "arm",
	"amd64": "x86_64",
	"386":   "x86",
}

// DeviceArch 返回当前设备/宿主的规范架构字符串
func DeviceArch() string {
	if a, ok := archByGOARCH[runtime.GOARCH]; ok {
		return a
	}
	return "unknown"
}

type ELFInfo struct {
	Arch string // arm64|arm|x86_64|x86|unknown
	Type string // exec|dyn|unknown
}

var archByMachine = map[elf.Machine]string{
	elf.EM_AARCH64: "arm64",
	elf.EM_ARM:     "arm",
	elf.EM_X86_64:  "x86_64",
	elf.EM_386:     "x86",
}

func Inspect(r io.ReaderAt) (ELFInfo, error) {
	f, err := elf.NewFile(r)
	if err != nil {
		return ELFInfo{}, fmt.Errorf("%w: %v", errNotELF, err)
	}
	defer f.Close()
	arch := archByMachine[f.Machine]
	if arch == "" {
		arch = "unknown"
	}
	typ := "unknown"
	switch f.Type {
	case elf.ET_EXEC:
		typ = "exec"
	case elf.ET_DYN:
		typ = "dyn"
	}
	return ELFInfo{Arch: arch, Type: typ}, nil
}

func InspectFile(path string) (ELFInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return ELFInfo{}, err
	}
	defer f.Close()
	return Inspect(f)
}
