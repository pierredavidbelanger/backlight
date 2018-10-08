package main

import (
	"fmt"
	"github.com/urfave/cli"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var deviceFile string
var deviceActualBrightnessFile string
var deviceMaxBrightnessFile string
var deviceBrightnessFile string
var deviceLatestBrightnessFile string

func main() {

	app := cli.NewApp()
	app.Name = "backlight"
	app.Usage = "get or set backlight"
	app.Version = "v1.0.0"
	app.ErrWriter = os.Stderr

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "file, f",
			Usage:       "The device `FILE` (something like /sys/class/backlight/intel_backlight)",
			Destination: &deviceFile,
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "get",
			Aliases: []string{"g"},
			Usage:   "get the actual backlight value",
			Action:  actionGet,
		},
		{
			Name:    "restore",
			Aliases: []string{"r"},
			Usage:   "restore the last known backlight value",
			Action:  actionRestore,
		},
		{
			Name:      "set",
			Aliases:   []string{"s"},
			Usage:     "set the new backlight value",
			Action:    actionSet,
			ArgsUsage: "n[%]",
		},
		{
			Name:      "inc",
			Aliases:   []string{"i"},
			Usage:     "increment the new backlight value",
			Action:    actionInc,
			ArgsUsage: "n[%]",
		},
		{
			Name:      "dec",
			Aliases:   []string{"d"},
			Usage:     "decrement the new backlight value",
			Action:    actionDec,
			ArgsUsage: "n[%]",
		},
	}

	app.Before = func(c *cli.Context) error {

		if deviceFile == "" {

			root := "/sys/class/backlight"
			filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

				if path == root {
					return nil
				}

				if err != nil {
					return err
				}

				deviceFile = path

				return filepath.SkipDir
			})

		}

		if deviceFile == "" {
			return fmt.Errorf("%s should point to a device folder", deviceFile)
		}

		info, err := fileMustExists(deviceFile, fmt.Sprintf("%s must be an existing device folder", deviceFile))
		if err != nil {
			return err
		}

		if !info.IsDir() {
			return fmt.Errorf("%s should point to a device folder", deviceFile)
		}

		deviceActualBrightnessFile = filepath.Join(deviceFile, "actual_brightness")
		_, err = fileMustExists(deviceActualBrightnessFile, "device's actual_brightness file must exists")
		if err != nil {
			return err
		}

		deviceMaxBrightnessFile = filepath.Join(deviceFile, "max_brightness")
		_, err = fileMustExists(deviceMaxBrightnessFile, "device's max_brightness file must exists")
		if err != nil {
			return err
		}

		deviceBrightnessFile = filepath.Join(deviceFile, "brightness")
		_, err = fileMustExists(deviceBrightnessFile, "device's brightness file must exists")
		if err != nil {
			return err
		}

		deviceName := filepath.Base(deviceFile)

		usr, err := user.Current()
		if err != nil {
			return err
		}
		deviceLatestBrightnessFile = filepath.Join(usr.HomeDir, ".cache", "backlight", deviceName)
		deviceLatestBrightnessDiv := filepath.Dir(deviceLatestBrightnessFile)
		err = os.MkdirAll(deviceLatestBrightnessDiv, os.ModePerm)
		if err != nil {
			return err
		}

		return nil
	}

	err := app.Run(os.Args)

	if err != nil {
		log.Fatal(err)
	}
}

func actionGet(c *cli.Context) error {
	return get(c, deviceActualBrightnessFile, deviceLatestBrightnessFile)
}

func actionRestore(c *cli.Context) (err error) {
	return get(c, deviceLatestBrightnessFile, deviceBrightnessFile)
}

func actionSet(c *cli.Context) error {
	return set(c, 0)
}

func actionInc(c *cli.Context) error {
	return set(c, 1)
}

func actionDec(c *cli.Context) error {
	return set(c, -1)
}

func fileMustExists(filePath string, message string) (os.FileInfo, error) {
	info, err := os.Stat(deviceFile)
	if err != nil {
		return info, fmt.Errorf("%s: %s", message, err)
	}
	return info, nil
}

func get(c *cli.Context, readFile, writeFile string) (err error) {

	actual, err := read(readFile)
	if err != nil {
		return
	}

	max, err := read(deviceMaxBrightnessFile)
	if err != nil {
		return nil
	}

	err = write(writeFile, actual)
	if err != nil {
		return err
	}

	fmt.Printf("device:%s\nactual:%d\nmax:%d\n", deviceFile, actual, max)

	return nil
}

func set(c *cli.Context, action int) (err error) {

	value, percent, err := parseValueArg(c)
	if err != nil {
		return
	}

	actual, err := read(deviceActualBrightnessFile)
	if err != nil {
		return
	}

	max, err := read(deviceMaxBrightnessFile)
	if err != nil {
		return
	}

	if percent {
		if value <= 0 {
			value = 0
		} else if value >= 100 {
			value = max
		} else {
			value = (max / 100) * value
		}
	}

	if action < 0 {
		actual -= value
	} else if action == 0 {
		actual = value
	} else {
		actual += value
	}

	if actual < 0 {
		actual = 0
	} else if actual > max {
		actual = max
	}

	err = write(deviceBrightnessFile, actual)
	if err != nil {
		return
	}

	return actionGet(c)
}

func parseValueArg(c *cli.Context) (value int, percent bool, err error) {

	if c.NArg() != 1 {
		err = fmt.Errorf("a value to set is needed")
		return
	}

	arg := c.Args().First()

	pattern := regexp.MustCompile("(\\d+)(%?)")

	args := pattern.FindStringSubmatch(arg)
	if args == nil {
		err = fmt.Errorf("a valid value to set is needed")
		return
	}

	value, err = strconv.Atoi(args[1])
	if err != nil {
		return
	}

	percent = args[2] == "%"

	return
}

func read(file string) (value int, err error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return
	}
	value, err = strconv.Atoi(strings.TrimSpace(string(bytes)))
	if err != nil {
		return
	}
	return
}

func write(file string, actual int) error {
	data := fmt.Sprintf("%s\n", strconv.Itoa(actual))
	return ioutil.WriteFile(file, []byte(data), 0644)
}
