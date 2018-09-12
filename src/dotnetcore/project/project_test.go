package project_test

import (
	"dotnetcore/project"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -source=project.go --destination=mocks_project_test.go --package=project_test

var _ = Describe("Project", func() {
	var (
		err          error
		buildDir     string
		depsDir      string
		depsIdx      string
		subject      *project.Project
		mockCtrl     *gomock.Controller
		mockManifest *MockManifest
	)

	BeforeEach(func() {
		buildDir, err = ioutil.TempDir("", "dotnet-core-buildpack.build.")
		Expect(err).To(BeNil())

		depsDir, err = ioutil.TempDir("", "dotnetcore-buildpack.deps.")
		Expect(err).To(BeNil())

		depsIdx = "9"
		Expect(os.MkdirAll(filepath.Join(depsDir, depsIdx), 0755)).To(Succeed())

		mockCtrl = gomock.NewController(GinkgoT())
		mockManifest = NewMockManifest(mockCtrl)
		subject = project.New(buildDir, filepath.Join(depsDir, depsIdx), depsIdx, mockManifest)
	})

	AfterEach(func() {
		err = os.RemoveAll(buildDir)
		Expect(err).To(BeNil())
	})

	Describe("StartCommand", func() {
		Context("The project is published", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "fred.runtimeconfig.json"), []byte(""), 0644)).To(Succeed())
			})

			Context("An executable for the project exists", func() {
				//before: make a 'fred' executable.
				BeforeEach(func() {
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "fred"), []byte(""), 0755)).To(Succeed())
				})

				It("returns ${HOME}/project", func() {
					startCmd, err := subject.StartCommand()
					Expect(err).To(BeNil())
					Expect(startCmd).To(Equal(filepath.Join("${HOME}", "fred")))
				})
			})

			Context("An executable for the project does NOT exist, but a dll does", func() {
				BeforeEach(func() {
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "fred.dll"), []byte(""), 0755)).To(Succeed())
				})

				It("returns ${HOME}/project.dll", func() {
					startCmd, err := subject.StartCommand()
					Expect(err).To(BeNil())
					Expect(startCmd).To(Equal(filepath.Join("${HOME}", "fred.dll")))
				})
			})
			Context("An executable for the project does NOT exist, and neither does a dll", func() {
				It("returns an empty string", func() {
					startCmd, err := subject.StartCommand()
					Expect(err).To(BeNil())
					Expect(startCmd).To(Equal(""))
				})
			})
		})

		Context("The project is NOT published", func() {
			Context("The csproj file does not have an AssemblyName tag", func() {
				BeforeEach(func() {
					Expect(os.MkdirAll(filepath.Join(buildDir, "subdir"), 0755)).To(Succeed())
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "subdir", "fred.csproj"), []byte("<Project></Project>"), 0644)).To(Succeed())
					Expect(os.MkdirAll(filepath.Join(depsDir, depsIdx, "dotnet_publish"), 0755)).To(Succeed())
				})

				Context("An executable for the project exists", func() {
					BeforeEach(func() {
						Expect(ioutil.WriteFile(filepath.Join(depsDir, depsIdx, "dotnet_publish", "fred"), []byte(""), 0755)).To(Succeed())
					})

					It("returns ${DEPS_DIR}/DepsIdx/project", func() {
						startCmd, err := subject.StartCommand()
						Expect(err).To(BeNil())
						Expect(startCmd).To(Equal(filepath.Join("${DEPS_DIR}", depsIdx, "dotnet_publish", "fred")))
					})
				})

				Context("An executable for the project does NOT exist, but a dll does", func() {
					BeforeEach(func() {
						Expect(ioutil.WriteFile(filepath.Join(depsDir, depsIdx, "dotnet_publish", "fred.dll"), []byte(""), 0755)).To(Succeed())
					})

					It("returns ${DEPS_DIR}/DepsIdx/project.dll", func() {
						startCmd, err := subject.StartCommand()
						Expect(err).To(BeNil())
						Expect(startCmd).To(Equal(filepath.Join("${DEPS_DIR}", depsIdx, "dotnet_publish", "fred.dll")))
					})

				})

				Context("An executable for the project does NOT exist, and neither does a dll", func() {
					It("returns an empty string", func() {
						startCmd, err := subject.StartCommand()
						Expect(err).To(BeNil())
						Expect(startCmd).To(Equal(""))
					})
				})
			})

			Context("The csproj file has an AssemblyName tag", func() {
				BeforeEach(func() {
					Expect(os.MkdirAll(filepath.Join(buildDir, "subdir"), 0755)).To(Succeed())
					csprojContents := `
<Project Sdk="Microsoft.NET.Sdk.Web">
	<PropertyGroup>
		<AssemblyName>f.red.csproj</AssemblyName>
	</PropertyGroup>
</Project>`
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "subdir", "fred.csproj"), []byte(csprojContents), 0644)).To(Succeed())
					Expect(os.MkdirAll(filepath.Join(depsDir, depsIdx, "dotnet_publish"), 0755)).To(Succeed())
					Expect(ioutil.WriteFile(filepath.Join(depsDir, depsIdx, "dotnet_publish", "f.red"), []byte(""), 0755)).To(Succeed())
				})

				It("returns a start command with the AssemblyName instead of filename", func() {
					startCmd, err := subject.StartCommand()
					Expect(err).To(BeNil())
					Expect(startCmd).To(Equal(filepath.Join("${DEPS_DIR}", depsIdx, "dotnet_publish", "f.red")))
				})
			})
		})

		Context("mainPath could be determined", func() {
			BeforeEach(func() {
				Expect(os.MkdirAll(filepath.Join(depsDir, depsIdx, "dotnet_publish"), 0755)).To(Succeed())
			})

			It("returns an empty string", func() {
				startCmd, err := subject.StartCommand()
				Expect(err).To(BeNil())
				Expect(startCmd).To(Equal(""))
			})
		})
	})

	Describe("DeploymentType", func() {
		Context("when a runtime config with framework is present", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "foo.runtimeconfig.json"),
					[]byte(`{ "runtimeOptions": { "framework": { "name": "Microsoft.NETCore.App", "version": "4.5.6" }, "applyPatches": false } }`), 0644)).To(Succeed())
			})

			It("Returns FDD", func() {
				deploymentType, err := subject.DeploymentType()
				Expect(err).To(BeNil())
				Expect(deploymentType).To(Equal("FDD"))
			})
		})

		Context("when a runtime config with no framework is present", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "foo.runtimeconfig.json"),
					[]byte(`{ "runtimeOptions": { "applyPatches": false } }`), 0644)).To(Succeed())
			})

			It("Returns SCD", func() {
				deploymentType, err := subject.DeploymentType()
				Expect(err).To(BeNil())
				Expect(deploymentType).To(Equal("SCD"))
			})
		})

		Context("when there is no runtime config", func() {
			It("Returns SOURCE", func() {
				deploymentType, err := subject.DeploymentType()
				Expect(err).To(BeNil())
				Expect(deploymentType).To(Equal("SOURCE"))
			})
		})
	})

	Describe("GetVersionFromDepsJSON", func() {
		Context("when a .deps.json does contain aspnetcore.app", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "foo.deps.json"),
					[]byte(`{ "libraries": { "Microsoft.AspNetCore.App/2.1.1": { "name": "Microsoft.NETCore.App", "version": "4.5.6" } } }`), 0644)).To(Succeed())
			})

			It("Returns the associated version", func() {
				version, err := subject.GetVersionFromDepsJSON()
				Expect(err).To(BeNil())
				Expect(version).To(Equal("2.1.1"))
			})
		})

		Context("when a .deps.json does not contain aspnetcore.app", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "foo.deps.json"),
					[]byte(`{ "libraries": { "Microsoft.Foo.App/2.1.1": { "name": "Microsoft.NETCore.App", "version": "4.5.6" } } }`), 0644)).To(Succeed())
			})

			It("returns an error", func() {
				_, err := subject.GetVersionFromDepsJSON()
				Expect(err).Should(MatchError("Deps.json file didn't contain Microsoft.AspNetCore.App key"))
			})
		})

		Context("when a .deps.json is not present", func() {
			It("returns an error", func() {
				_, err := subject.GetVersionFromDepsJSON()
				Expect(err).Should(MatchError("Multiple or no .deps.json files present"))
			})
		})
	})

	Describe("FindMatchingFrameworkVersion", func() {
		Context("when applyPatches is false", func() {
			applyPatches := false

			Context("and the manifest has the exact version", func() {
				BeforeEach(func() {
					mockManifest.EXPECT().AllDependencyVersions("dotnet-runtime").Return([]string{"4.5.6", "4.5.7"})
				})

				It("Returns the exact version", func() {
					version, err := subject.FindMatchingFrameworkVersion("dotnet-runtime", "4.5.6", &applyPatches)
					Expect(err).To(BeNil())
					Expect(version).To(Equal("4.5.6"))
				})
			})
		})

		Context("when applyPatches is true", func() {
			applyPatches := true

			BeforeEach(func() {
				mockManifest.EXPECT().AllDependencyVersions("dotnet-runtime").Return([]string{"4.5.6", "4.5.7"})
			})

			It("Returns the same major.minor version with the highest available patch", func() {
				version, err := subject.FindMatchingFrameworkVersion("dotnet-runtime", "4.5.6", &applyPatches)
				Expect(err).To(BeNil())
				Expect(version).To(Equal("4.5.7"))
			})
		})
	})

	Describe("VersionFromProjFile", func() {
		var runtimeRegex, aspnetcoreRegex string

		BeforeEach(func() {
			runtimeRegex = "<RuntimeFrameworkVersion>(.*)</RuntimeFrameworkVersion>"
			aspnetcoreRegex = `"Microsoft.AspNetCore.All" Version="(.*)"`

		})

		Context("When looking for dotnet-aspnetcore version", func() {
			Context("when aspnetcore is specified in the proj file", func() {
				BeforeEach(func() {
					csprojXml := `<Project Sdk="Microsoft.NET.Sdk.Web">
												<ItemGroup>
												  <PackageReference Include="Microsoft.AspNetCore.All" Version="2.0.*" />
												</ItemGroup>
										</Project>`
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "test_app.csproj"), []byte(csprojXml), 0644)).To(Succeed())
				})

				It("returns the aspnet version", func() {
					mockManifest.EXPECT().AllDependencyVersions("dotnet-aspnetcore").Return([]string{"2.0.0", "2.0.3"})
					aspnetcoreVersion, err := subject.VersionFromProjFile(filepath.Join(buildDir, "test_app.csproj"), aspnetcoreRegex, "dotnet-aspnetcore")
					Expect(err).To(BeNil())
					Expect(aspnetcoreVersion).To(Equal("2.0.3"))
				})
			})

			Context("when aspnetcore is NOT specified in the proj file", func() {
				BeforeEach(func() {
					csprojXml := `<Project Sdk="Microsoft.NET.Sdk.Web">
												<PropertyGroup>
													<RuntimeFrameworkVersion>2.1.2</RuntimeFrameworkVersion>
												</PropertyGroup>
										</Project>`
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "test_app.csproj"), []byte(csprojXml), 0644)).To(Succeed())
				})

				It("returns an empty string", func() {
					mockManifest.EXPECT().AllDependencyVersions("dotnet-aspnetcore").Return([]string{"2.0.0", "2.0.3"})
					aspnetcoreVersion, err := subject.VersionFromProjFile(filepath.Join(buildDir, "test_app.csproj"), aspnetcoreRegex, "dotnet-aspnetcore")
					Expect(err).To(BeNil())
					Expect(aspnetcoreVersion).To(Equal(""))
				})
			})
		})

		Context("When looking for dotnet-runtime version", func() {
			Context("when runtime is specified in the proj file", func() {
				BeforeEach(func() {
					csprojXml := `<Project Sdk="Microsoft.NET.Sdk">
													<RuntimeFrameworkVersion>2.1.2</RuntimeFrameworkVersion>
										</Project>`
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "test_app.csproj"), []byte(csprojXml), 0644)).To(Succeed())
				})
				It("returns the runtime version", func() {
					mockManifest.EXPECT().AllDependencyVersions("dotnet-runtime").Return([]string{"2.1.2"})
					runtimeVersion, err := subject.VersionFromProjFile(filepath.Join(buildDir, "test_app.csproj"), runtimeRegex, "dotnet-runtime")
					Expect(err).To(BeNil())
					Expect(runtimeVersion).To(Equal("2.1.2"))
				})
			})

			Context("when runtime is NOT specified in the proj file", func() {
				BeforeEach(func() {
					csprojXml := `<Project Sdk="Microsoft.NET.Sdk.Web">
												<PropertyGroup>
												</PropertyGroup>
										</Project>`
					Expect(ioutil.WriteFile(filepath.Join(buildDir, "test_app.csproj"), []byte(csprojXml), 0644)).To(Succeed())
				})

				It("returns an empty string", func() {
					mockManifest.EXPECT().AllDependencyVersions("dotnet-runtime").Return([]string{"2.0.0", "2.0.3"})
					runtimeVersion, err := subject.VersionFromProjFile(filepath.Join(buildDir, "test_app.csproj"), runtimeRegex, "dotnet-runtime")
					Expect(err).To(BeNil())
					Expect(runtimeVersion).To(Equal(""))
				})
			})
		})
	})

	Describe("ProjFilePaths", func() {
		BeforeEach(func() {
			for _, name := range []string{
				"first.csproj",
				"other.txt",
				"dir/second.csproj",
				".cloudfoundry/other.csproj",
				"dir/other.txt",
				"a/b/first.vbproj",
				"b/c/first.fsproj",
				"c/d/other.txt",
			} {
				Expect(os.MkdirAll(filepath.Dir(filepath.Join(buildDir, name)), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(buildDir, name), []byte(""), 0644)).To(Succeed())
			}
		})

		It("returns csproj, fsproj and vbproj files (excluding .cloudfoundry)", func() {
			Expect(subject.ProjFilePaths()).To(ConsistOf([]string{
				filepath.Join(buildDir, "first.csproj"),
				filepath.Join(buildDir, "dir", "second.csproj"),
				filepath.Join(buildDir, "a", "b", "first.vbproj"),
				filepath.Join(buildDir, "b", "c", "first.fsproj"),
			}))
		})
	})

	Describe("IsPublished", func() {
		BeforeEach(func() {
			for _, name := range []string{
				"first.csproj",
				"c/d/other.txt",
			} {
				Expect(os.MkdirAll(filepath.Dir(filepath.Join(buildDir, name)), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(buildDir, name), []byte(""), 0644)).To(Succeed())
			}
		})

		Context("*.runtimeconfig.json exists", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "fred.runtimeconfig.json"), []byte(""), 0644)).To(Succeed())
			})

			It("returns true", func() {
				Expect(subject.IsPublished()).To(BeTrue())
			})
		})
		Context("*.runtimeconfig.json does NOT exist", func() {
			It("returns false", func() {
				Expect(subject.IsPublished()).To(BeFalse())
			})
		})
	})

	Describe("IsFsharp", func() {
		BeforeEach(func() {
			for _, name := range []string{
				"first.csproj",
				"c/d/other.txt",
			} {
				Expect(os.MkdirAll(filepath.Dir(filepath.Join(buildDir, name)), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(buildDir, name), []byte(""), 0644)).To(Succeed())
			}
		})

		Context(".fsproj file exists", func() {
			BeforeEach(func() {
				name := "a/c/something.fsproj"
				Expect(os.MkdirAll(filepath.Dir(filepath.Join(buildDir, name)), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(buildDir, name), []byte(""), 0644)).To(Succeed())
			})

			It("returns true", func() {
				Expect(subject.IsFsharp()).To(BeTrue())
			})
		})
		Context(".fsproj file does NOT exist", func() {
			It("returns false", func() {
				Expect(subject.IsFsharp()).To(BeFalse())
			})
		})
		Context(".fsproj file exists inside deps directory (.cloudfoundry)", func() {
			BeforeEach(func() {
				name := ".cloudfoundry/0/a/b/something.fsproj"
				Expect(os.MkdirAll(filepath.Dir(filepath.Join(buildDir, name)), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(buildDir, name), []byte(""), 0644)).To(Succeed())
			})

			It("returns false", func() {
				Expect(subject.IsFsharp()).To(BeFalse())
			})
		})
	})

	Describe("MainPath", func() {
		Context("There is a runtimeconfig file present", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "fred.runtimeconfig.json"), []byte(""), 0644)).To(Succeed())
			})

			It("returns the runtimeconfig file", func() {
				configFile, err := subject.MainPath()
				Expect(err).To(BeNil())
				Expect(configFile).To(Equal(filepath.Join(buildDir, "fred.runtimeconfig.json")))
			})
		})

		Context("No project path in paths", func() {
			It("returns an empty string", func() {
				path, err := subject.MainPath()
				Expect(err).To(BeNil())
				Expect(path).To(Equal(""))
			})
		})

		Context("Exactly one project path in paths", func() {
			BeforeEach(func() {
				Expect(os.MkdirAll(filepath.Join(buildDir, "subdir"), 0755)).To(Succeed())
				Expect(ioutil.WriteFile(filepath.Join(buildDir, "subdir", "first.csproj"), []byte(""), 0644)).To(Succeed())
			})
			It("returns that one path", func() {
				path, err := subject.MainPath()
				Expect(err).To(BeNil())
				Expect(path).To(Equal(filepath.Join(buildDir, "subdir", "first.csproj")))
			})
		})

		Context("More than one project path in paths", func() {
			BeforeEach(func() {
				for _, name := range []string{
					"first.csproj",
					"other.txt",
					"dir/second.csproj",
					".cloudfoundry/other.csproj",
					"dir/other.txt",
					"a/b/first.vbproj",
					"b/c/first.fsproj",
					"c/d/other.txt",
				} {
					Expect(os.MkdirAll(filepath.Dir(filepath.Join(buildDir, name)), 0755)).To(Succeed())
					Expect(ioutil.WriteFile(filepath.Join(buildDir, name), []byte(""), 0644)).To(Succeed())
				}
			})

			Context("There is a .deployment file present", func() {
				BeforeEach(func() {
					Expect(ioutil.WriteFile(filepath.Join(buildDir, ".deployment"), []byte("[config]\nproject = ./a/b/first.vbproj"), 0644)).To(Succeed())
				})
				It("returns the path specified in the .deployment file.", func() {
					path, err := subject.MainPath()
					Expect(err).To(BeNil())
					Expect(path).To(Equal(filepath.Join(buildDir, "a", "b", "first.vbproj")))
				})
			})

			Context("There is NOT a .deployment file present", func() {

				It("Returns an error", func() {
					_, err := subject.MainPath()
					Expect(err).ToNot(BeNil())
				})
			})
		})
	})
})
