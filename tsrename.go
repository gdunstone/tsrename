package main

import (
	"os"
	"fmt"
	"flag"
	"path"
	"path/filepath"
	"regexp"
	"time"
	"errors"
	"io"
	"github.com/rwcarlsen/goexif/exif"
)

const (
	tsForm         = "2006_01_02_15_04_05"
	dumbExifForm   = "2006:01:02 15:04:05"
	tsDirStruct    = "2006/2006_01/2006_01_02/2006_01_02_15/"
	tsRegexPattern = "[0-9][0-9][0-9][0-9]_[0-9][0-9]_[0-9][0-9]_[0-9][0-9]_[0-9][0-9]_[0-9][0-9]"
)

var (
	rootDir, outputDir, namedOutput string
	del                             bool
	datetimeFunc                    datetimeFunction
)

var /* const */ tsRegex = regexp.MustCompile(tsRegexPattern)

func Printfln(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func moveFilebyCopy(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	if del {
		absSrc, _ := filepath.Abs(src)
		absDest, _ := filepath.Abs(dst)
		if absSrc != absDest {
			os.Remove(src)
		}
	}
	return d.Close()
}

type datetimeFunction func(string) (time.Time, error)

func getTimeFromExif(thisFile string) (time.Time, error) {
	fileHandler, err := os.Open(thisFile)
	if err != nil {
		// file wouldnt open
		return time.Time{}, err
	}

	exifData, err := exif.Decode(fileHandler)
	if err != nil {
		// exif wouldnt decode
		return time.Time{}, err
	}

	dt, err := exifData.Get(exif.DateTime) // normally, don't ignore errors!
	if err != nil {
		// couldnt get DateTime from exif
		return time.Time{}, err
	}

	datetimeString, err := dt.StringVal()
	if err != nil {
		// couldnt get
		return time.Time{}, err
	}
	thisTime, err := time.Parse(dumbExifForm, datetimeString)
	if err != nil {
		return time.Time{}, err
	}
	return thisTime, nil
}

func getTimeFromFileTimestamp(thisFile string) (time.Time, error) {
	timestamp := tsRegex.FindString(thisFile)
	if len(timestamp) < 1 {
		// no timestamp found in filename
		return time.Time{}, errors.New("failed regex timestamp from filename")
	}

	t, err := time.Parse(tsForm, timestamp)
	if err != nil {
		// parse error
		return time.Time{}, err
	}
	return t, nil
}

func parseFilename(thisFile string) (string, error) {
	thisTime, err := datetimeFunc(thisFile)
	if err != nil {
		return "", err
	}

	formattedSubdirs := thisTime.Format(tsDirStruct)
	targetFilename := path.Base(thisFile)

	if len(namedOutput) > 0 {
		ext := path.Ext(targetFilename)
		targetFilename = namedOutput + "_" + thisTime.Format(tsForm) + ext
	}

	newT := path.Join(outputDir, formattedSubdirs, targetFilename)

	return newT, nil
}

func visit(filePath string, info os.FileInfo, err error) error {
	// skip directories
	if info.IsDir() {
		return nil
	}

	// parse the new filepath
	newPath, err := parseFilename(filePath)
	if err != nil {
		Printfln("[parse] %s", err)
		return nil
	}

	// make directories
	err = os.MkdirAll(path.Dir(newPath), 0755)
	if err != nil {
		Printfln("[mkdir] %s", err)
		return nil
	}

	absSrc, _ := filepath.Abs(filePath)
	absDest, _ := filepath.Abs(newPath)
	if absSrc == absDest {
		Printfln("[dupe] %s", newPath)
		return nil
	}

	// rename/copy+del if del is true otherwise moveFilebyCopy to not del.
	if del {
		err = os.Rename(filePath, newPath)
		if err != nil {
			err = moveFilebyCopy(filePath, newPath)
		}
	} else {
		err = moveFilebyCopy(filePath, newPath)
	}
	if err != nil {
		Printfln("[move] %s", err)
		return nil
	}

	fmt.Println(newPath)
	return nil
}

var usage = func() {
	fmt.Fprintf(os.Stderr, "usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\tcopy into structure:\n")
	fmt.Fprintf(os.Stderr, "\t\t %s <source>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\tcopy into structure at <destination>:\n")
	fmt.Fprintf(os.Stderr, "\t\t %s <source> -output=<destination>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\tcopy into structure with <name> prefix:\n")
	fmt.Fprintf(os.Stderr, "\t\t %s <source> -name=<name>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\trename (move) into structure:\n")
	fmt.Fprintf(os.Stderr, "\t\t %s <source> -del\n", os.Args[0])

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "flags:\n")
	fmt.Fprintf(os.Stderr, "\t-del: removes the source files\n")
	fmt.Fprintf(os.Stderr, "\t-name: renames the prefix fo the target files\n")
	fmt.Fprintf(os.Stderr, "\t-exit: uses exif data to rename rather than file timestamp\n")
	pwd, _ := os.Getwd()
	fmt.Fprintf(os.Stderr, "\t-output: set the <destination> directory (default=%s)\n", pwd)
}

func init() {
	flagset := flag.NewFlagSet("", flag.ExitOnError)

	flagset.Usage = usage
	flag.Usage = usage
	// set flags for flagset
	flagset.StringVar(&namedOutput, "name", "", "name for the stream")
	flagset.StringVar(&outputDir, "output", "", "output directory")
	flagset.BoolVar(&del, "del", false, "delete source files")
	useExif := flagset.Bool("exif", false, "use exif instead of timestamps in filenames")
	// parse the leading argument with normal flag.Parse
	flag.Parse()
	if flag.NArg() < 1 {
		Printfln("[path] no <source> specified")
		usage()
		os.Exit(1)
	}
	// parse flags using a flagset, ignore the first 2 (first arg is program name)
	flagset.Parse(os.Args[2:])

	if *useExif {
		datetimeFunc = getTimeFromExif
	} else {
		datetimeFunc = getTimeFromFileTimestamp
	}

	rootDir = flag.Arg(0)

}

func main() {

	if _, err := os.Stat(rootDir); err != nil {
		if os.IsNotExist(err) {
			Printfln("[path] <source> %s does not exist.", rootDir)
			os.Exit(1)
		}
	}
	if outputDir == "" {
		outputDir = rootDir
		Printfln("[path] no <destination>, creating %s", outputDir)
		os.MkdirAll(outputDir, 0755)
	}

	if err := filepath.Walk(rootDir, visit); err != nil {
		Printfln("[walk] %s", err)
	}
	//c := make(chan error)
	//go func() {
	//	c <- filepath.Walk(rootDir, visit)
	//}()
	//
	//if err := <-c; err != nil {
	//	fmt.Println(err)
	//}
}
