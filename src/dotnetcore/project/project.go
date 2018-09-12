package project

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudfoundry/libbuildpack"
	"github.com/go-ini/ini"
)

type Manifest interface {
	AllDependencyVersions(string) []string
}
type Project struct {
	buildDir string
	depDir   string
	depsIdx  string
	manifest Manifest
}

type ConfigJSON struct {
	RuntimeOptions struct {
		Framework struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"framework"`
		ApplyPatches *bool `json:"applyPatches"`
	} `json:"runtimeOptions"`
}

func New(buildDir, depDir, depsIdx string, manifest Manifest) *Project {
	return &Project{buildDir: buildDir, depDir: depDir, depsIdx: depsIdx, manifest: manifest}
}

func (p *Project) IsPublished() (bool, error) {
	if path, err := p.RuntimeConfigFile(); err != nil {
		return false, err
	} else {
		return path != "", nil
	}
}

func (p *Project) StartCommand() (string, error) {
	projectPath, err := p.MainPath()
	if err != nil {
		return "", err
	} else if projectPath == "" {
		return "", nil
	}
	runtimeConfigRe := regexp.MustCompile(`\.(runtimeconfig\.json)$`)
	projRe := regexp.MustCompile(`\.([a-z]+proj)$`)

	if runtimeConfigRe.MatchString(projectPath) {
		projectPath = runtimeConfigRe.ReplaceAllString(projectPath, "")
		projectPath = filepath.Base(projectPath)
	} else if projRe.MatchString(projectPath) {
		assemblyName, err := p.getAssemblyName(projectPath)
		if err != nil {
			return "", err
		}
		if assemblyName != "" {
			projectPath = projRe.ReplaceAllString(assemblyName, "")
		} else {
			projectPath = projRe.ReplaceAllString(projectPath, "")
			projectPath = filepath.Base(projectPath)
		}
	}

	return p.publishedStartCommand(projectPath)
}

func (p *Project) ParseRuntimeConfig(runtimeConfig string) (ConfigJSON, error) {
	obj := ConfigJSON{}
	if err := libbuildpack.NewJSON().Load(runtimeConfig, &obj); err != nil {
		return ConfigJSON{}, err
	}
	return obj, nil
}

func (p *Project) DeploymentType() (string, error) {
	if path, err := p.RuntimeConfigFile(); err != nil {
		return "", err
	} else if path != "" {
		runtimeJSON, err := p.ParseRuntimeConfig(path)
		if err != nil {
			return "", err
		}
		if runtimeJSON.RuntimeOptions.Framework.Name != "" {
			return "FDD", nil
		} else {
			return "SCD", nil
		}
	}
	return "SOURCE", nil
}

func (p *Project) FindMatchingFrameworkVersion(name, version string, applyPatches *bool) (string, error) {
	// TODO: implement roll forward behavior (https://github.com/dotnet/core-setup/blob/master/Documentation/design-docs/roll-forward-on-no-candidate-fx.md)
	//	add another argument for roll-forward-on-no-candidate-fx (int: 0, 1, 2)
	//
	var err error
	if applyPatches == nil || *applyPatches {
		version, err = p.getLatestPatch(name, version)
		if err != nil {
			return "", err
		}
	}
	return version, nil
}

func (p *Project) GetVersionFromDepsJSON() (string, error) {
	// TODO: consider refactoring the bit where the path is found to make this and RuntimeConfigFile() call the same function.
	depsJSONFiles, err := filepath.Glob(filepath.Join(p.buildDir, "*.deps.json"))
	if err != nil {
		return "", err
	}
	if len(depsJSONFiles) == 1 {
		depsBytes, err := ioutil.ReadFile(depsJSONFiles[0])
		if err != nil {
			return "", err
		}

		var result map[string]interface{}
		json.Unmarshal(depsBytes, &result)
		libraries := result["libraries"].(map[string]interface{})
		for key := range libraries {
			re := regexp.MustCompile(`(Microsoft.AspNetCore.App)\/(\d\.\d\.\d)`)
			matchedString := re.FindStringSubmatch(key)
			if matchedString != nil {
				return matchedString[2], nil
			}
		}
		return "", fmt.Errorf("Deps.json file didn't contain Microsoft.AspNetCore.App key")
	}
	return "", fmt.Errorf("Multiple or no .deps.json files present")

}

func (p *Project) VersionFromProjFile(mainProjectFile, regex, name string) (string, error) {
	proj, err := ioutil.ReadFile(mainProjectFile)
	if err != nil {
		return "", err
	}

	r := regexp.MustCompile(regex)
	matches := r.FindStringSubmatch(string(proj))
	//TODO: Check if version can ever be just *. Currently that wouldn't work here.
	version := ""
	if len(matches) > 1 {
		version = matches[1]
		if version[len(version)-1] == '*' {
			return p.getLatestPatch(name, version)
		}
	}
	return version, nil
}

func (p *Project) ProjFilePaths() ([]string, error) {
	paths := []string{}
	if err := filepath.Walk(p.buildDir, func(path string, _ os.FileInfo, err error) error {
		if strings.Contains(path, "/.cloudfoundry/") {
			return filepath.SkipDir
		}
		if strings.HasSuffix(path, ".csproj") || strings.HasSuffix(path, ".vbproj") || strings.HasSuffix(path, ".fsproj") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return []string{}, err
	}
	return paths, nil
}

func (p *Project) IsFsharp() (bool, error) {
	if paths, err := p.ProjFilePaths(); err != nil {
		return false, err
	} else {
		for _, path := range paths {
			if strings.HasSuffix(path, ".fsproj") {
				return true, nil
			}
		}
	}
	return false, nil
}

func (p *Project) RuntimeConfigFile() (string, error) {
	if configFiles, err := filepath.Glob(filepath.Join(p.buildDir, "*.runtimeconfig.json")); err != nil {
		return "", err
	} else if len(configFiles) == 1 {
		return configFiles[0], nil
	} else if len(configFiles) > 1 {
		return "", fmt.Errorf("Multiple .runtimeconfig.json files present")
	}
	return "", nil
}

func (p *Project) MainPath() (string, error) {
	if runtimeConfigFile, err := p.RuntimeConfigFile(); err != nil {
		return "", err
	} else if runtimeConfigFile != "" {
		return runtimeConfigFile, nil
	}
	paths, err := p.ProjFilePaths()
	if err != nil {
		return "", err
	}

	if len(paths) == 1 {
		return paths[0], nil
	} else if len(paths) > 1 {
		if exists, err := libbuildpack.FileExists(filepath.Join(p.buildDir, ".deployment")); err != nil {
			return "", err
		} else if exists {
			deployment, err := ini.Load(filepath.Join(p.buildDir, ".deployment"))
			if err != nil {
				return "", err
			}
			config, err := deployment.GetSection("config")
			if err != nil {
				return "", err
			}
			project, err := config.GetKey("project")
			if err != nil {
				return "", err
			}
			return filepath.Join(p.buildDir, strings.Trim(project.String(), ".")), nil
		}
		return "", fmt.Errorf("Multiple paths: %v contain a project file, but no .deployment file was used", paths)
	}
	return "", nil
}

func (p *Project) publishedStartCommand(projectPath string) (string, error) {
	var publishedPath string
	var runtimePath string

	if published, err := p.IsPublished(); err != nil {
		return "", err
	} else if published {
		publishedPath = p.buildDir
		runtimePath = "${HOME}"
	} else {
		publishedPath = filepath.Join(p.depDir, "dotnet_publish")
		runtimePath = filepath.Join("${DEPS_DIR}", p.depsIdx, "dotnet_publish")
	}

	if exists, err := libbuildpack.FileExists(filepath.Join(publishedPath, projectPath)); err != nil {
		return "", err
	} else if exists {
		if err := os.Chmod(filepath.Join(filepath.Join(publishedPath, projectPath)), 0755); err != nil {
			return "", err
		}
		return filepath.Join(runtimePath, projectPath), nil
	}

	if exists, err := libbuildpack.FileExists(filepath.Join(publishedPath, fmt.Sprintf("%s.dll", projectPath))); err != nil {
		return "", fmt.Errorf("checking if a %s.dll file exists: %v", projectPath, err)
	} else if exists {
		return fmt.Sprintf("%s.dll", filepath.Join(runtimePath, projectPath)), nil
	}
	return "", nil
}

func (p *Project) getAssemblyName(projectPath string) (string, error) {
	projFile, err := os.Open(projectPath)
	if err != nil {
		return "", err
	}
	defer projFile.Close()
	projBytes, err := ioutil.ReadAll(projFile)
	if err != nil {
		return "", err
	}

	proj := struct {
		PropertyGroup struct {
			AssemblyName string
		}
	}{}
	err = xml.Unmarshal(projBytes, &proj)
	if err != nil {
		return "", err
	}
	return proj.PropertyGroup.AssemblyName, nil
}

func (p *Project) getLatestPatch(name, version string) (string, error) {
	v := strings.Split(version, ".")
	v[2] = "x"
	versions := p.manifest.AllDependencyVersions(name)
	latestPatch, err := libbuildpack.FindMatchingVersion(strings.Join(v, "."), versions)
	if err != nil {
		return "", err
	}
	return latestPatch, nil
}
