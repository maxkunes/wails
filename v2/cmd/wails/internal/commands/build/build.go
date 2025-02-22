package build

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/wailsapp/wails/v2/internal/colour"
	"github.com/wailsapp/wails/v2/internal/project"
	"github.com/wailsapp/wails/v2/internal/system"

	"github.com/wailsapp/wails/v2/cmd/wails/internal"
	"github.com/wailsapp/wails/v2/internal/gomod"

	"github.com/leaanthony/clir"
	"github.com/leaanthony/slicer"
	"github.com/wailsapp/wails/v2/pkg/clilogger"
	"github.com/wailsapp/wails/v2/pkg/commands/build"
)

// AddBuildSubcommand adds the `build` command for the Wails application
func AddBuildSubcommand(app *clir.Cli, w io.Writer) {

	outputType := "desktop"

	validTargetTypes := slicer.String([]string{"desktop", "hybrid", "server"})

	command := app.NewSubCommand("build", "Builds the application")

	// Setup noPackage flag
	noPackage := false
	command.BoolFlag("noPackage", "Skips platform specific packaging", &noPackage)

	compilerCommand := "go"
	command.StringFlag("compiler", "Use a different go compiler to build, eg go1.15beta1", &compilerCommand)

	skipModTidy := false
	command.BoolFlag("m", "Skip mod tidy before compile", &skipModTidy)

	compress := false
	command.BoolFlag("upx", "Compress final binary with UPX (if installed)", &compress)

	compressFlags := ""
	command.StringFlag("upxflags", "Flags to pass to upx", &compressFlags)

	// Setup Platform flag
	platform := runtime.GOOS + "/"
	if system.IsAppleSilicon {
		platform += "arm64"
	} else {
		platform += runtime.GOARCH
	}

	command.StringFlag("platform", "Platform to target. Comma separate multiple platforms", &platform)

	// Verbosity
	verbosity := 1
	command.IntFlag("v", "Verbosity level (0 - silent, 1 - default, 2 - verbose)", &verbosity)

	// ldflags to pass to `go`
	ldflags := ""
	command.StringFlag("ldflags", "optional ldflags", &ldflags)

	// tags to pass to `go`
	tags := ""
	command.StringFlag("tags", "tags to pass to Go compiler (quoted and space separated)", &tags)

	outputFilename := ""
	command.StringFlag("o", "Output filename", &outputFilename)

	// Clean build directory
	cleanBuildDirectory := false
	command.BoolFlag("clean", "Clean the build directory before building", &cleanBuildDirectory)

	webview2 := "download"
	command.StringFlag("webview2", "WebView2 installer strategy: download,embed,browser,error.", &webview2)

	skipFrontend := false
	command.BoolFlag("s", "Skips building the frontend", &skipFrontend)

	forceBuild := false
	command.BoolFlag("f", "Force build application", &forceBuild)

	updateGoMod := false
	command.BoolFlag("u", "Updates go.mod to use the same Wails version as the CLI", &updateGoMod)

	debug := false
	command.BoolFlag("debug", "Retains debug data in the compiled application", &debug)

	command.Action(func() error {

		quiet := verbosity == 0

		// Create logger
		logger := clilogger.New(w)
		logger.Mute(quiet)

		// Validate output type
		if !validTargetTypes.Contains(outputType) {
			return fmt.Errorf("output type '%s' is not valid", outputType)
		}

		if !quiet {
			app.PrintBanner()
		}

		// Lookup compiler path
		compilerPath, err := exec.LookPath(compilerCommand)
		if err != nil {
			return fmt.Errorf("unable to find compiler: %s", compilerCommand)
		}

		// Tags
		experimental := false
		userTags := []string{}
		for _, tag := range strings.Split(tags, " ") {
			thisTag := strings.TrimSpace(tag)
			if thisTag != "" {
				userTags = append(userTags, thisTag)
			}
			if thisTag == "exp" {
				experimental = true
			}
		}

		if runtime.GOOS == "linux" && !experimental {
			return fmt.Errorf("Linux version coming soon!")
		}

		// Webview2 installer strategy (download by default)
		wv2rtstrategy := ""
		webview2 = strings.ToLower(webview2)
		if webview2 != "" {
			validWV2Runtime := slicer.String([]string{"download", "embed", "browser", "error"})
			if !validWV2Runtime.Contains(webview2) {
				return fmt.Errorf("invalid option for flag 'webview2': %s", webview2)
			}
			// These are the build tags associated with the strategies
			switch webview2 {
			case "embed":
				wv2rtstrategy = "wv2runtime.embed"
			case "error":
				wv2rtstrategy = "wv2runtime.error"
			case "browser":
				wv2rtstrategy = "wv2runtime.browser"
			}
		}

		mode := build.Production
		modeString := "Production"
		if debug {
			mode = build.Debug
			modeString = "Debug"
		}

		var targets slicer.StringSlicer
		targets.AddSlice(strings.Split(platform, ","))
		targets.Deduplicate()

		// Create BuildOptions
		buildOptions := &build.Options{
			Logger:              logger,
			OutputType:          outputType,
			OutputFile:          outputFilename,
			CleanBuildDirectory: cleanBuildDirectory,
			Mode:                mode,
			Pack:                !noPackage,
			LDFlags:             ldflags,
			Compiler:            compilerCommand,
			SkipModTidy:         skipModTidy,
			Verbosity:           verbosity,
			ForceBuild:          forceBuild,
			IgnoreFrontend:      skipFrontend,
			Compress:            compress,
			CompressFlags:       compressFlags,
			UserTags:            userTags,
			WebView2Strategy:    wv2rtstrategy,
		}

		// Start a new tabwriter
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 8, 8, 0, '\t', 0)

		// Write out the system information
		fmt.Fprintf(w, "App Type: \t%s\n", buildOptions.OutputType)
		fmt.Fprintf(w, "Platforms: \t%s\n", platform)
		fmt.Fprintf(w, "Compiler: \t%s\n", compilerPath)
		fmt.Fprintf(w, "Build Mode: \t%s\n", modeString)
		fmt.Fprintf(w, "Skip Frontend: \t%t\n", skipFrontend)
		fmt.Fprintf(w, "Compress: \t%t\n", buildOptions.Compress)
		fmt.Fprintf(w, "Package: \t%t\n", buildOptions.Pack)
		fmt.Fprintf(w, "Clean Build Dir: \t%t\n", buildOptions.CleanBuildDirectory)
		fmt.Fprintf(w, "LDFlags: \t\"%s\"\n", buildOptions.LDFlags)
		fmt.Fprintf(w, "Tags: \t[%s]\n", strings.Join(buildOptions.UserTags, ","))
		if len(buildOptions.OutputFile) > 0 && targets.Length() == 1 {
			fmt.Fprintf(w, "Output File: \t%s\n", buildOptions.OutputFile)
		}
		fmt.Fprintf(w, "\n")
		w.Flush()

		err = checkGoModVersion(logger, updateGoMod)
		if err != nil {
			return err
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		projectOptions, err := project.Load(cwd)

		// Check platform
		validPlatformArch := slicer.String([]string{
			"darwin",
			"darwin/amd64",
			"darwin/arm64",
			"darwin/universal",
			"linux",
			"linux/amd64",
			"linux/arm64",
			"windows",
			"windows/amd64",
			"windows/arm64",
		})

		targets.Each(func(platform string) {

			if !validPlatformArch.Contains(platform) {
				buildOptions.Logger.Println("platform '%s' is not supported - skipping. Supported platforms: %s", platform, validPlatformArch.Join(","))
				return
			}

			desiredFilename := projectOptions.OutputFilename
			if desiredFilename == "" {
				desiredFilename = projectOptions.Name
			}
			desiredFilename = strings.TrimSuffix(desiredFilename, ".exe")

			// Calculate platform and arch
			platformSplit := strings.Split(platform, "/")
			buildOptions.Platform = platformSplit[0]
			if system.IsAppleSilicon {
				buildOptions.Arch = "arm64"
			} else {
				buildOptions.Arch = runtime.GOARCH
			}
			if len(platformSplit) == 2 {
				buildOptions.Arch = platformSplit[1]
			}

			banner := "Building target: " + platform
			logger.Println(banner)
			logger.Println(strings.Repeat("-", len(banner)))

			if compress && platform == "darwin/universal" {
				logger.Println("Warning: compress flag unsupported for universal binaries. Ignoring.")
				compress = false
			}

			switch buildOptions.Platform {
			case "linux":
				if runtime.GOOS != "linux" {
					logger.Println("Crosscompiling to Linux not currently supported.\n")
					return
				}
			case "darwin":
				if runtime.GOOS != "darwin" {
					logger.Println("Crosscompiling to Mac not currently supported.\n")
					return
				}
				macTargets := targets.Filter(func(platform string) bool {
					return strings.HasPrefix(platform, "darwin")
				})
				if macTargets.Length() == 2 {
					buildOptions.BundleName = fmt.Sprintf("%s-%s.app", desiredFilename, buildOptions.Arch)
				}
			}

			if targets.Length() > 1 {
				// target filename
				switch buildOptions.Platform {
				case "windows":
					desiredFilename = fmt.Sprintf("%s-%s", desiredFilename, buildOptions.Arch)
				case "linux", "darwin":
					desiredFilename = fmt.Sprintf("%s-%s-%s", desiredFilename, buildOptions.Platform, buildOptions.Arch)
				}
			}
			if buildOptions.Platform == "windows" {
				desiredFilename += ".exe"
			}
			buildOptions.OutputFile = desiredFilename

			// Start Time
			start := time.Now()

			outputFilename, err := build.Build(buildOptions)
			if err != nil {
				logger.Println("Error: ", err.Error())
				return
			}

			// Subsequent iterations
			buildOptions.IgnoreFrontend = true
			buildOptions.CleanBuildDirectory = false

			// Output stats
			buildOptions.Logger.Println(fmt.Sprintf("Built '%s' in %s.\n", outputFilename, time.Since(start).Round(time.Millisecond).String()))

		})
		return nil
	})
}

func checkGoModVersion(logger *clilogger.CLILogger, updateGoMod bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	gomodFilename := filepath.Join(cwd, "go.mod")
	gomodData, err := os.ReadFile(gomodFilename)
	if err != nil {
		return err
	}
	outOfSync, err := gomod.GoModOutOfSync(gomodData, internal.Version)
	if err != nil {
		return err
	}
	if !outOfSync {
		return nil
	}
	gomodversion, err := gomod.GetWailsVersionFromModFile(gomodData)
	if err != nil {
		return err
	}

	if updateGoMod {
		return syncGoModVersion(cwd)
	}

	logger.Println("Warning: go.mod is using Wails '%s' but the CLI is '%s'. Consider updating your project's `go.mod` file.\n", gomodversion.String(), internal.Version)
	return nil
}

func LogGreen(message string, args ...interface{}) {
	text := fmt.Sprintf(message, args...)
	println(colour.Green(text))
}

func syncGoModVersion(cwd string) error {
	gomodFilename := filepath.Join(cwd, "go.mod")
	gomodData, err := os.ReadFile(gomodFilename)
	if err != nil {
		return err
	}
	outOfSync, err := gomod.GoModOutOfSync(gomodData, internal.Version)
	if err != nil {
		return err
	}
	if !outOfSync {
		return nil
	}
	LogGreen("Updating go.mod to use Wails '%s'", internal.Version)
	newGoData, err := gomod.UpdateGoModVersion(gomodData, internal.Version)
	if err != nil {
		return err
	}
	return os.WriteFile(gomodFilename, newGoData, 0755)
}
