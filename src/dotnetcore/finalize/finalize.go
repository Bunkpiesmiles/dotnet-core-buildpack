package finalize

import (
	"dotnetcore/config"
	"dotnetcore/project"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/kr/text"
)

var cfStackToOS = map[string]string{
	"cflinuxfs2":  "ubuntu.14.04-x64",
	"cflinuxfs3":  "ubuntu.18.04-x64",
	"cflinuxfs3m": "ubuntu.18.04-x64",
}

type Installer interface {
	FetchDependency(libbuildpack.Dependency, string) error
	InstallDependency(libbuildpack.Dependency, string) error
	InstallOnlyVersion(string, string) error
}
type Stager interface {
	BuildDir() string
	DepsIdx() string
	DepDir() string
	WriteProfileD(string, string) error
}

type Command interface {
	Run(*exec.Cmd) error
}

type DotnetRuntime interface {
	Install(string) error
}

type DotnetAspNetCore interface {
	Install(string) error
}

type Finalizer struct {
	Stager           Stager
	Log              *libbuildpack.Logger
	Command          Command
	DotnetRuntime    DotnetRuntime
	DotnetAspNetCore DotnetAspNetCore
	Config           *config.Config
	Project          *project.Project
	Installer        Installer
}

func Run(f *Finalizer) error {
	f.Log.BeginStep("Finalizing Dotnet Core")

	if err := f.DotnetRestore(); err != nil {
		f.Log.Error("Unable to run dotnet restore: %s", err.Error())
		return err
	}

	// mainPath, err := f.Project.MainPath()
	// if err != nil {
	// 	return err
	// }

	// if err := f.DotnetRuntime.Install(mainPath); err != nil {
	// 	f.Log.Error("Unable to install required dotnet runtime: %s", err.Error())
	// 	return err
	// }
	//
	// if err := f.DotnetAspNetCore.Install(mainPath); err != nil {
	// 	f.Log.Error("Unable to install required dotnet aspnetcore: %s", err.Error())
	// 	return err
	// }
	//
	if err := f.InstallFrameworks(); err != nil {
		f.Log.Error("Unable to install frameworks: %s", err.Error())
		return err
	}

	if err := f.DotnetPublish(); err != nil {
		f.Log.Error("Unable to run dotnet publish: %s", err.Error())
		return err
	}

	if err := f.CleanStagingArea(); err != nil {
		f.Log.Error("Unable to run CleanStagingArea: %s", err.Error())
		return err
	}

	if err := f.WriteProfileD(); err != nil {
		f.Log.Error("Unable to write profile.d: %s", err.Error())
		return err
	}

	data, err := f.GenerateReleaseYaml()
	if err != nil {
		f.Log.Error("Error generating release YAML: %s", err)
		return err
	}
	releasePath := filepath.Join(f.Stager.BuildDir(), "tmp", "dotnet-core-buildpack-release-step.yml")
	return libbuildpack.NewYAML().Write(releasePath, data)
}

func (f *Finalizer) InstallFrameworks() error {
	// Source Deployment
	//    cs proj
	// Self contained
	//    nuget (?)
	// ParseRuntimeConfig
	// act based on name of framework:

	deploymentType, err := f.Project.DeploymentType()
	if err != nil {
		return err
	}

	var aspnetcoreVersion, runtimeVersion string
	if deploymentType == "FDD" {
		aspnetcoreVersion, runtimeVersion, err = f.FDDFrameworkVersions()
		if err != nil {
			return err
		}
	} else if deploymentType == "SOURCE" {
		aspnetcoreVersion, runtimeVersion, err = f.SourceFrameworkVersions()
		if err != nil {
			return err
		}

	}

	// else if isSCD {
	// 	return f.SCDInstallFrameworks()
	// } else {
	// 	return f.SourceInstallFrameworks()
	// }
	if err := f.Installer.InstallDependency(libbuildpack.Dependency{Name: "dotnet-aspnetcore", Version: aspnetcoreVersion}, filepath.Join(f.Stager.DepDir(), "dotnet-sdk")); err != nil {
		return err
	}
	if err := f.Installer.InstallDependency(libbuildpack.Dependency{Name: "dotnet-runtime", Version: runtimeVersion}, filepath.Join(f.Stager.DepDir(), "dotnet-sdk")); err != nil {
		return err
	}
	return nil
}

func (f *Finalizer) SourceFrameworkVersions() (string, string, error) {
	mainPath, err := f.Project.MainPath()
	if err != nil {
		return "", "", err
	}
	runtimeRegex := "<RuntimeFrameworkVersion>(*)</RuntimeFrameworkVersion>"
	aspnetcoreRegex := `"Microsoft.AspNetCore.All" Version="(.*)"`
	runtimeVersion, err := f.Project.VersionFromProjFile(mainPath, runtimeRegex, "dotnet-runtime")
	if err != nil {
		return "", "", err
	}
	aspnetcoreVersion, err := f.Project.VersionFromProjFile(mainPath, aspnetcoreRegex, "dotnet-aspnetcore")
	if err != nil {
		return "", "", err
	}

	return runtimeVersion, aspnetcoreVersion, nil
}
func (f *Finalizer) FDDFrameworkVersions() (string, string, error) {
	applicationRuntimeConfig, err := f.Project.RuntimeConfigFile()
	if err != nil {
		return "", "", err
	}
	configJSON, err := f.Project.ParseRuntimeConfig(applicationRuntimeConfig)
	if err != nil {
		return "", "", err
	}

	var aspnetcoreVersion, runtimeVersion string

	frameworkName := configJSON.RuntimeOptions.Framework.Name
	frameworkVersion := configJSON.RuntimeOptions.Framework.Version
	applyPatches := configJSON.RuntimeOptions.ApplyPatches

	if frameworkName == "Microsoft.AspNetCore.All" || frameworkName == "Microsoft.AspNetCore.App" {
		aspnetcoreVersion, err := f.Project.FindMatchingFrameworkVersion("dotnet-aspnetcore", frameworkVersion, applyPatches)
		if err != nil {
			return "", "", err
		}
		aspnetConfigJSON, err := f.Project.ParseRuntimeConfig(filepath.Join(f.Stager.DepDir(), "dotnet-sdk", "shared", "Microsoft.AspNetCore.App", aspnetcoreVersion, "Microsoft.AspNetCore.App.runtimeconfig.json"))
		if err != nil {
			return "", "", err
		}
		runtimeVersion, err = f.Project.FindMatchingFrameworkVersion("dotnet-runtime", aspnetConfigJSON.RuntimeOptions.Framework.Version, applyPatches)
		if err != nil {
			return "", "", err
		}

	} else if frameworkName == "Microsoft.NETCore.App" {
		runtimeVersion, err = f.Project.FindMatchingFrameworkVersion("dotnet-runtime", frameworkVersion, applyPatches)
		if err != nil {
			return "", "", err
		}
		aspnetcoreVersion, err = f.Project.GetVersionFromDepsJSON()
		if err != nil {
			return "", "", err
		}
	} else {
		return "", "", fmt.Errorf("invalid framework specified in application runtime config file")
	}
	return aspnetcoreVersion, runtimeVersion, nil

}

func (f *Finalizer) CleanStagingArea() error {
	f.Log.BeginStep("Cleaning staging area")

	dirsToRemove := []string{"nuget", ".nuget", ".local", ".cache", ".config", ".npm"}

	if startCmd, err := f.Project.StartCommand(); err != nil {
		return err
	} else if !strings.HasSuffix(startCmd, ".dll") {
		dirsToRemove = append(dirsToRemove, "dotnet-sdk")
	}
	if os.Getenv("INSTALL_NODE") != "true" {
		dirsToRemove = append(dirsToRemove, "node")
	}

	for _, dir := range dirsToRemove {
		if found, err := libbuildpack.FileExists(filepath.Join(f.Stager.DepDir(), dir)); err != nil {
			return err
		} else if found {
			f.Log.Info("Removing %s", dir)
			if err := os.RemoveAll(filepath.Join(f.Stager.DepDir(), dir)); err != nil {
				return err
			}
			if err := f.removeSymlinksTo(filepath.Join(f.Stager.DepDir(), dir)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *Finalizer) removeSymlinksTo(dir string) error {
	for _, name := range []string{"bin", "lib"} {
		files, err := ioutil.ReadDir(filepath.Join(f.Stager.DepDir(), name))
		if err != nil {
			return err
		}

		for _, file := range files {
			if (file.Mode() & os.ModeSymlink) != 0 {
				source := filepath.Join(f.Stager.DepDir(), name, file.Name())
				target, err := os.Readlink(source)
				if err != nil {
					return err
				}
				if strings.HasPrefix(target, dir) {
					if err := os.Remove(source); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (f *Finalizer) WriteProfileD() error {
	scriptContents := "export ASPNETCORE_URLS=http://0.0.0.0:${PORT}\n"

	return f.Stager.WriteProfileD("startup.sh", scriptContents)
}

func (f *Finalizer) GenerateReleaseYaml() (map[string]map[string]string, error) {
	startCmd, err := f.Project.StartCommand()
	if err != nil {
		return nil, err
	}
	directory := filepath.Dir(startCmd)
	startCmd = "./" + filepath.Base(startCmd)
	if strings.HasSuffix(startCmd, ".dll") {
		startCmd = "dotnet " + startCmd
	}
	return map[string]map[string]string{
		"default_process_types": {"web": fmt.Sprintf("cd %s && %s --server.urls http://0.0.0.0:${PORT}", directory, startCmd)},
	}, nil
}

func (f *Finalizer) DotnetRestore() error {
	if published, err := f.Project.IsPublished(); err != nil {
		return err
	} else if published {
		return nil
	}
	f.Log.BeginStep("Restore dotnet dependencies")
	env := f.shellEnvironment()
	paths, err := f.Project.ProjFilePaths()
	if err != nil {
		return err
	}
	for _, path := range paths {
		cmd := exec.Command("dotnet", "restore", path)
		cmd.Dir = f.Stager.BuildDir()
		cmd.Env = env
		cmd.Stdout = indentWriter(os.Stdout)
		cmd.Stderr = indentWriter(os.Stderr)
		if err := f.Command.Run(cmd); err != nil {
			return err
		}
	}
	return nil
}

func (f *Finalizer) DotnetPublish() error {
	if published, err := f.Project.IsPublished(); err != nil {
		return err
	} else if published {
		return nil
	}
	f.Log.BeginStep("Publish dotnet")

	mainProject, err := f.Project.MainPath()
	if err != nil {
		return err
	}

	env := f.shellEnvironment()
	env = append(env, "PATH="+filepath.Join(filepath.Dir(mainProject), "node_modules", ".bin")+":"+os.Getenv("PATH"))

	publishPath := filepath.Join(f.Stager.DepDir(), "dotnet_publish")
	if err := os.MkdirAll(publishPath, 0755); err != nil {
		return err
	}
	args := []string{"publish", mainProject, "-o", publishPath, "-c", f.publicConfig()}
	if strings.HasPrefix(f.Config.DotnetSdkVersion, "2.") {
		args = append(args, "-r", cfStackToOS[os.Getenv("CF_STACK")])
	}
	cmd := exec.Command("dotnet", args...)
	cmd.Dir = f.Stager.BuildDir()
	cmd.Env = env
	cmd.Stdout = indentWriter(os.Stdout)
	cmd.Stderr = indentWriter(os.Stderr)

	f.Log.Debug("Running command: %v", cmd)
	if err := f.Command.Run(cmd); err != nil {
		return err
	}

	return nil
}

func (f *Finalizer) publicConfig() string {
	if os.Getenv("PUBLISH_RELEASE_CONFIG") == "true" {
		return "Release"
	}

	return "Debug"
}

func (f *Finalizer) shellEnvironment() []string {
	env := os.Environ()
	for _, v := range []string{
		"DOTNET_SKIP_FIRST_TIME_EXPERIENCE=true",
		"DefaultItemExcludes=.cloudfoundry/**/*.*",
		"HOME=" + f.Stager.DepDir(),
	} {
		env = append(env, v)
	}
	return env
}

func indentWriter(writer io.Writer) io.Writer {
	return text.NewIndentWriter(writer, []byte("       "))
}
