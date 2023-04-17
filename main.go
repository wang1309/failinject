package failinject

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wang1309/failinject/code"
)

var (
	// Git SHA Value will be set during build
	GitSHA = "Not provided (use ./build.sh instead of go build)"

	Version = "0.1.0"
)

var usageLine = `Usage:
gofail enable [list of files or directories]
    Enable the failpoints

gofail disable [list of files or directories]
    Disable the checkpoints
	
gofail --version
    Show the version of gofail`

type xfrmFunc func(io.Writer, io.Reader) ([]*code.Failpoint, error)

func xfrmFile(xfrm xfrmFunc, path string) ([]*code.Failpoint, error) {
	src, serr := os.Open(path)
	if serr != nil {
		return nil, serr
	}
	defer src.Close()

	dst, derr := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if derr != nil {
		return nil, derr
	}
	defer dst.Close()

	fps, xerr := xfrm(dst, src)
	if xerr != nil || len(fps) == 0 {
		os.Remove(dst.Name())
		return nil, xerr
	}

	rerr := os.Rename(dst.Name(), path)
	if rerr != nil {
		os.Remove(dst.Name())
		return nil, rerr
	}

	return fps, nil
}

func dir2files(dir, ext string) (ret []string, err error) {
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, err
	}

	f, ferr := os.Open(dir)
	if ferr != nil {
		return nil, ferr
	}
	defer f.Close()

	names, rerr := f.Readdirnames(0)
	if rerr != nil {
		return nil, rerr
	}
	for _, f := range names {
		if path.Ext(f) != ext {
			continue
		}
		ret = append(ret, path.Join(dir, f))
	}
	return ret, nil
}

func paths2files(paths []string) (files []string) {
	// no paths => use cwd
	if len(paths) == 0 {
		wd, gerr := os.Getwd()
		if gerr != nil {
			fmt.Println(gerr)
			os.Exit(1)
		}
		return paths2files([]string{wd})
	}
	for _, p := range paths {
		s, serr := os.Stat(p)
		if serr != nil {
			fmt.Println(serr)
			os.Exit(1)
		}
		if s.IsDir() {
			fs, err := dir2files(p, ".go")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			files = append(files, fs...)
		} else if path.Ext(s.Name()) == ".go" {
			abs, err := filepath.Abs(p)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			files = append(files, abs)
		}
	}
	return files
}

func writeBinding(file string, fps []*code.Failpoint) {
	if len(fps) == 0 {
		return
	}
	fname := strings.Split(path.Base(file), ".go")[0] + ".fail.go"
	out, err := os.Create(path.Join(path.Dir(file), fname))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// XXX: support "package main"
	pkgAbsDir := path.Dir(file)
	pkg := path.Base(pkgAbsDir)
	code.NewBinding(pkg, fps).Write(out)
	out.Close()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usageLine)
		os.Exit(1)
	}

	var xfrm xfrmFunc
	enable := false
	switch os.Args[1] {
	case "enable":
		xfrm = code.ToFailpoints
		enable = true
	case "disable":
		xfrm = code.ToComments
	case "--version":
		showVersion()
	default:
		fmt.Println(usageLine)
		os.Exit(1)
	}

	files := paths2files(os.Args[2:])
	fps := [][]*code.Failpoint{}
	for _, path := range files {
		curfps, err := xfrmFile(xfrm, path)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fps = append(fps, curfps)
	}

	if enable {
		// build runtime bindings <FILE>.fail.go
		for i := range files {
			writeBinding(files[i], fps[i])
		}
	} else {
		// remove all runtime bindings
		for i := range files {
			fname := strings.Split(path.Base(files[i]), ".go")[0] + ".fail.go"
			os.Remove(path.Join(path.Dir(files[i]), fname))
		}
	}
}

func showVersion() {
	fmt.Println("Git SHA: ", GitSHA)
	fmt.Println("Go Version: ", runtime.Version())
	fmt.Println("gofail Version: ", Version)
	fmt.Printf("Go OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(0)
}
