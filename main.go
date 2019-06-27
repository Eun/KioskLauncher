package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32              = syscall.MustLoadDLL("user32.dll")
	procEnumWindows     = user32.MustFindProc("EnumWindows")
	procGetWindowTextW  = user32.MustFindProc("GetWindowTextW")
	procShowWindow      = user32.MustFindProc("ShowWindow")
	procIsWindowVisible = user32.MustFindProc("IsWindowVisible")
)

func EnumWindows(enumFunc uintptr, lparam uintptr) (err error) {
	r1, _, e1 := syscall.Syscall(procEnumWindows.Addr(), 2, uintptr(enumFunc), uintptr(lparam), 0)
	if r1 == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func GetWindowText(hwnd syscall.Handle, str *uint16, maxCount int32) (len int32, err error) {
	r0, _, e1 := syscall.Syscall(procGetWindowTextW.Addr(), 3, uintptr(hwnd), uintptr(unsafe.Pointer(str)), uintptr(maxCount))
	len = int32(r0)
	if len == 0 {
		if e1 != 0 {
			err = error(e1)
		} else {
			err = syscall.EINVAL
		}
	}
	return
}

func FindWindow(title string) ([]syscall.Handle, error) {
	var hwnds []syscall.Handle
	cb := syscall.NewCallback(func(h syscall.Handle, p uintptr) uintptr {
		b := make([]uint16, 200)
		_, err := GetWindowText(h, &b[0], int32(len(b)))
		if err != nil {
			// ignore the error
			return 1 // continue enumeration
		}

		if syscall.UTF16ToString(b) == title {
			if IsWindowVisible(h) {
				hwnds = append(hwnds, h)
			}
		}
		return 1 // continue enumeration
	})
	EnumWindows(cb, 0)
	return hwnds, nil
}

func IsWindowVisible(hWnd syscall.Handle) bool {
	ret, _, _ := syscall.Syscall(procIsWindowVisible.Addr(), 1,
		uintptr(hWnd),
		0,
		0)

	return ret != 0
}

func ShowWindow(hwnd syscall.Handle, nCmdShow int) bool {
	r0, _, _ := syscall.Syscall(procShowWindow.Addr(), 2, uintptr(hwnd), uintptr(nCmdShow), 0)
	return r0 != 0
}

func start(path string, args ...string) *os.Process {
	log.Printf("Starting `%s'...\n", path)
	var procAttr os.ProcAttr
	procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}

	process, err := os.StartProcess(path, append([]string{path}, args...), &procAttr)
	if err != nil {
		log.Fatal(err.Error())
	}

	return process
}

func waitForWindow(title string) (hwnds []syscall.Handle) {
	var err error
	for {
		log.Printf("Waiting for `%s'...\n", title)
		hwnds, err = FindWindow(title)
		if err != nil {
			log.Fatal(err.Error())
		}
		if len(hwnds) > 0 {
			log.Printf("Found '%s': %v\n", title, hwnds)
			return hwnds
		}
		time.Sleep(time.Second)
	}
}

func main() {
	ex, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	content, err := ioutil.ReadFile(filepath.Join(filepath.Dir(ex), "config.json"))
	if err != nil {
		log.Fatal(err)
	}
	type Config struct {
		Program struct {
			Path      string
			Arguments []string
			Title     string
		}
		After struct {
			Path      string
			Arguments []string
		}
	}
	var cfg Config
	if err = json.Unmarshal(content, &cfg); err != nil {
		log.Fatal(err)
	}
	proc := start(cfg.Program.Path, cfg.Program.Arguments...)
	hwnds := waitForWindow(cfg.Program.Title)
	for _, hwnd := range hwnds {
		log.Printf("Maximizing %v\n", hwnd)
		ShowWindow(hwnd, 3)
	}

	if cfg.After.Path == "" {
		os.Exit(0)
		return
	}

	log.Println("Waiting for process to exit")
	state, err := proc.Wait()
	if err != nil {
		log.Fatal(err)
	}
	if state.Exited() {
		log.Println("Process exited")
		proc = start(cfg.After.Path, cfg.After.Arguments...)
		if err = proc.Release(); err != nil {
			log.Fatal(err)
		}
	}

}
