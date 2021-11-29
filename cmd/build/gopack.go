package build

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/plentico/plenti/common"
)

var (
	// Regexp help:
	// () = brackets for grouping
	// \s = space
	// .* = any character
	// | = or statement
	// \n = newline
	// {0,} = repeat any number of times
	// \{ = just a closing curly bracket (escaped)

	// Match dynamic import statments, e.g. import("") or import('').
	reDynamicImport = regexp.MustCompile(`import\((?:'|").*(?:'|")\)`)
	// Find any import statement in the file (including multiline imports).
	reStaticImportGoPk = regexp.MustCompile(`(?m)^import(\s)(.*from(.*);|((.*\n){0,})\}(\s)from(.*);)`)
	// Find all export statements.
	reStaticExportGoPk = regexp.MustCompile(`export(\s)(.*from(.*);|((.*\n){0,})\}(\s)from(.*);)`)
	// Find the path specifically (part between single or double quotes).
	rePath = regexp.MustCompile(`(?:'|").*(?:'|")`)
)

// Gopack ensures ESM support for NPM dependencies.
func Gopack(buildPath string) {

	defer Benchmark(time.Now(), "Running Gopack")

	Log("\nRunning gopack to build esm support for npm dependencies")

	// Start at the entry point for the app
	runPack(buildPath, buildPath+"/spa/ejected/main.js")

}

func runPack(buildPath, convertPath string, alreadyConvertedFiles ...string) error {

	if len(alreadyConvertedFiles) > 0 {
		for _, convertedFile := range alreadyConvertedFiles {
			if convertPath == convertedFile {
				// Exit the function to avoid endless loops where files
				// reference each other (like main.js and router.svelte)
				return nil
			}
		}
	}

	contentBytes, err := ioutil.ReadFile(convertPath)
	if err != nil {
		return fmt.Errorf("Could not read file %s to convert to esm: %w%s\n", convertPath, err, common.Caller())
	}

	// Created byte array of all dynamic imports in the current file.
	dynamicImportPaths := reDynamicImport.FindAll(contentBytes, -1)
	for _, dynamicImportPath := range dynamicImportPaths {
		// Inside the dynamic import change any svelte file extensions to reference regular javascript files.
		fixedImportPath := bytes.Replace(dynamicImportPath, []byte(".svelte"), []byte(".js"), 1)
		// Add the updated import back into the file contents for writing later.
		contentBytes = bytes.Replace(contentBytes, dynamicImportPath, fixedImportPath, 1)
	}

	// Get all the import statements.
	staticImportStatements := reStaticImportGoPk.FindAll(contentBytes, -1)
	// Get all the export statements.
	staticExportStatements := reStaticExportGoPk.FindAll(contentBytes, -1)
	// Combine import and export statements.
	allStaticStatements := append(staticImportStatements, staticExportStatements...)
	// Iterate through all static import and export statements.
	for _, staticStatement := range allStaticStatements {
		// Get path from the full import/export statement.
		pathBytes := rePath.Find(staticStatement)
		// Convert path to a string.
		pathStr := string(pathBytes)
		// Remove single or double quotes around path.
		pathStr = strings.Trim(pathStr, `'"`)
		// Intialize the path that we are replacing.
		var foundPath string

		// Convert .svelte file extensions to .js so the browser can read them.
		if filepath.Ext(pathStr) == ".svelte" {
			// Declare found since path should be relative to the component it's referencing.
			pathStr = strings.Replace(pathStr, ".svelte", ".js", 1)
		}

		// Make relative pathStr a full path that we can find on the filesystem.
		fullPathStr := path.Clean(path.Dir(convertPath) + "/" + pathStr)
		// Check that it exists (catches both converted files and those already in .js format)
		if fileExists(fullPathStr) {
			fmt.Println("fullpath: " + fullPathStr)
			fmt.Println("convertpath: " + convertPath)
			// Set this as a found path.
			foundPath = pathStr
			// Add the current file to list of already converted files.
			alreadyConvertedFiles = append(alreadyConvertedFiles, convertPath)
			// Use fullPathStr recursively to find its imports.
			runPack(buildPath, fullPathStr, alreadyConvertedFiles...)
		}

		// Make sure the import/export path doesn't start with a dot (.) or double dot (..)
		// and make sure that the path doesn't have a file extension.
		if pathStr[:1] != "." && filepath.Ext(pathStr) == "" {
			// Copy the npm file from /node_modules to /spa/web_modules
			copyNpmModule(pathStr, buildPath+"/spa/web_modules")
			// Try to connect the path to the file that was copied
			foundPath = checkNpmPath(buildPath, pathStr)
			// Make absolute foundPath relative to the current file so it works with baseurls.
			foundPath, err = filepath.Rel(path.Dir(convertPath), foundPath)
			if err != nil {
				fmt.Printf("Could not make path to NPM dependency relative: %s", err)
			}
		}

		if foundPath != "" {
			// Remove "public" build dir from path.
			replacePath := strings.Replace(foundPath, buildPath, "", 1)
			// Wrap path in quotes.
			replacePath = "'" + replacePath + "'"
			// Convert string path to bytes.
			replacePathBytes := []byte(replacePath)
			// Actually replace the path to the dependency in the source content.
			contentBytes = bytes.ReplaceAll(contentBytes, staticStatement,
				rePath.ReplaceAll(staticStatement, rePath.ReplaceAll(pathBytes, replacePathBytes)))
		} else {
			fmt.Printf("Import path '%s' not resolvable from file '%s'\n", pathStr, convertPath)
		}
	}
	// Overwrite the old file with the new content that contains the updated import path.
	err = ioutil.WriteFile(convertPath, contentBytes, 0644)
	if err != nil {
		return fmt.Errorf("Could not overwite %s with new import: %w%s\n", convertPath, err, common.Caller())
	}
	return nil

}

func checkNpmPath(buildPath, pathStr string) string {
	// A named import/export is being used, look for this in "web_modules/" dir.
	namedPath := buildPath + "/spa/web_modules/" + pathStr

	// Check all files in the current directory first.
	foundPath := findJSFile(namedPath)

	// our loop goes till we have no matching prefix in SeacrhPath so this is as far as that goes.
	if foundPath == "" {
		// If JS file was not found in the current directory, check nested directories.
		findSubPathErr := filepath.WalkDir(namedPath, func(subPath string, subPathFileInfo fs.DirEntry, err error) error {
			if err != nil {
				fmt.Printf("Can't walk path %s: %s\n", subPath, err)
			}
			// We've already checked all files, so look in next dir.
			if subPathFileInfo.IsDir() {
				// Check for any JS files at this dir level.
				foundPath = findJSFile(subPath)
				// Stop searching when a file is found
				if foundPath != "" {
					// Return a known error
					return io.EOF
				}

			}
			return nil
		})
		// Check for known error used to break out of walk
		if findSubPathErr == io.EOF {
			findSubPathErr = nil
		}
		// Check for real errors
		if findSubPathErr != nil {
			fmt.Printf("Could not find related .js file from named import: %s\n", findSubPathErr)
		}
	}
	return foundPath
}

// Checks for a JS file in the directory given.
func findJSFile(path string) string {

	var foundPath string
	files, err := os.ReadDir(path)
	if err != nil {
		fmt.Printf("Could not read files in dir '%s': %s\n", path, err)
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".js" || filepath.Ext(f.Name()) == ".mjs" {
			foundPath = path + "/" + f.Name()
		}
	}

	return foundPath
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		// The path was found on the filesystem
		return true
	}
	return false
}

func copyNpmModule(module string, gopackDir string) {
	// Walk through all sub directories of each dependency declared.
	nodeModuleErr := filepath.WalkDir("node_modules/"+module, func(modulePath string, moduleFileInfo fs.DirEntry, err error) error {

		if err != nil {
			return fmt.Errorf("can't stat %s: %w", modulePath, err)
		}
		// Only get ESM supported files.
		if !moduleFileInfo.IsDir() && filepath.Ext(modulePath) == ".mjs" {
			from, err := os.Open(modulePath)
			if err != nil {
				return fmt.Errorf("Could not open source .mjs %s file for copying: %w%s\n", modulePath, err, common.Caller())
			}
			defer from.Close()

			// Remove "node_modules" from path and add "web_modules".
			outPathFile := gopackDir + strings.Replace(modulePath, "node_modules", "", 1)

			// Create any subdirectories need to write file to "web_modules" destination.
			if err = os.MkdirAll(filepath.Dir(outPathFile), os.ModePerm); err != nil {
				return fmt.Errorf("Could not create subdirectories %s: %w%s\n", filepath.Dir(modulePath), err, common.Caller())
			}
			to, err := os.Create(outPathFile)
			if err != nil {
				return fmt.Errorf("Could not create destination %s file for copying: %w%s\n", modulePath, err, common.Caller())
			}
			defer to.Close()

			_, err = io.Copy(to, from)
			if err != nil {
				return fmt.Errorf("Could not copy .mjs  from source to destination: %w%s\n", err, common.Caller())
			}
		}
		return nil
	})
	if nodeModuleErr != nil {
		fmt.Printf("Could not get node module: %s\n", nodeModuleErr)
	}
}
