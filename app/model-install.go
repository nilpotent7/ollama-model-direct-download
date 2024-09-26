package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"runtime"
	"strings"
)

const (
	macOSModelsPath     = "~/.ollama/models"
	linuxOSModelsPath   = "/usr/share/ollama/.ollama/models"
	windowsOSModelsPath = "C:/Users/<username>/.ollama/models"

	manifestPattern = "manifests/<registry>/library/<model-name>"
	blobsPattern    = "blobs"
)

func getModelsPath() string {
	targetOS := runtime.GOOS
	customModelStorePath := os.Getenv("OLLAMA_MODELS")
	if customModelStorePath != "" {
		return strings.Replace(customModelStorePath, "%username%", os.Getenv("USERNAME"), 1)
	}
	switch targetOS {
	case "darwin":
		return macOSModelsPath
	case "linux":
		return linuxOSModelsPath
	case "windows":
		if os.Getenv("USERNAME") == "" {
			log.Println("Environment variable USERNAME is not set")
			os.Exit(1)
		}
		return strings.Replace(windowsOSModelsPath, "<username>", os.Getenv("USERNAME"), 1)
	default:
		fmt.Printf("OS %s not supported\n", targetOS)
		os.Exit(1)
	}
	return ""
}

func getManifestFile(folderPath string) (*os.File, error) {
	file, err := os.Open(path.Join(folderPath, "manifest"))
	if err != nil {
		return nil, fmt.Errorf("error opening manifest file: %v", err.Error())
	}
	return file, nil
}
func getBlobNames(path string) ([]string, error) {
	files, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("error reading blobs directory: %v", err.Error())
	}
	blobPaths := make([]string, 0)
	for _, file := range files {
		if file.IsDir() || file.Name() == "manifest" {
			continue
		}
		blobPaths = append(blobPaths, file.Name())
	}
	return blobPaths, nil
}

func parseBlobsDestinationPath(p string) string {
	newPath := strings.Replace(p, "\\", "/", -1)
	switch runtime.GOOS {
	case "windows":
		newPath = strings.Replace(newPath, "sha256", "sha256-", -1)
	default:
		newPath = strings.Replace(newPath, "sha256", "sha256:", 1)
	}
	return newPath
}

func parseModelName(modelName string) (string, string) {
	splitModelName := strings.Split(modelName, ":")
	modelTag := "latest"
	if len(splitModelName) == 2 {
		modelTag = splitModelName[1]
	} else {
		log.Println("Using default model tag if you encountered any error while using ollama try to install it with the acutal tag e.g. llama:16b ")
	}
	return modelTag, splitModelName[0]
}

func InstallModel(modelName string, downloadedModelPath string) error {
	modelsPath := getModelsPath()

	modelTag, modelPrefix := parseModelName(modelName)

	stat, err := os.Stat(modelsPath)
	if err != nil || !stat.IsDir() {
		log.Println(modelsPath)
		err := os.MkdirAll(modelsPath, 0755)
		if err != nil {
			return fmt.Errorf("error creating models directory: %v", err.Error())
		}
	}

	manifestPatternWithValues := strings.ReplaceAll(manifestPattern, "<registry>", DefaultRegistry)
	manifestPatternWithValues = strings.ReplaceAll(manifestPatternWithValues, "<model-name>", modelPrefix)
	manifestPath := path.Join(modelsPath, manifestPatternWithValues)

	manifestFolderStat, err := os.Stat(manifestPath)
	if err != nil && !errors.Is(os.ErrNotExist, err) {
		return fmt.Errorf("error checking manifest folder: %v", err.Error())
	}

	if manifestFolderStat == nil || !manifestFolderStat.IsDir() {
		err := os.MkdirAll(manifestPath, 0755)
		if err != nil {
			return fmt.Errorf("error creating manifest directory: %v", err.Error())
		}
	}
	_, err = os.Stat(path.Join(manifestPath, modelTag))
	modelAlreadyExists := err == nil
	if modelAlreadyExists {
		fmt.Print("\033[33m")
		fmt.Println("!Warning! Some Model Files already exists, Do you wish to override them ? this is permanent! Type 'Y' to proceed.")
		fmt.Print("\033[0m")

		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(strings.ToUpper(input))
		if input != "Y" {
			log.Println("Installation aborted")
			os.Exit(1)
		}
	}

	downloadedManifest, err := getManifestFile(downloadedModelPath)
	defer downloadedManifest.Close()
	if err != nil {
		return fmt.Errorf("error opening downloaded manifest file: %v", err.Error())
	}

	destinationManifestFile, err := os.Create(path.Join(manifestPath, modelTag))
	if err != nil {
		return err
	}
	defer destinationManifestFile.Close()

	_, err = io.Copy(destinationManifestFile, downloadedManifest)
	if err != nil {
		return fmt.Errorf("error copying downloaded manifest file: %v", err.Error())
	}

	err = destinationManifestFile.Sync()
	if err != nil {
		return fmt.Errorf("error syncing downloaded manifest file: %v", err.Error())
	}

	blobNames, err := getBlobNames(downloadedModelPath)
	if err != nil {
		return fmt.Errorf("error getting blobs path: %v", err.Error())
	}

	blobsFolderPath := path.Join(modelsPath, blobsPattern)
	if _, err := os.Stat(blobsPattern); err != nil {
		err = os.MkdirAll(blobsFolderPath, 0755)
		if err != nil {
			return fmt.Errorf("error creating blobs directory: %v", err.Error())
		}
	}
	log.Println("Copying blobs to", blobsFolderPath)
	log.Println("This may take a while so dont worry the program is not stuck")
	for _, blobName := range blobNames {
		blobFile, err := os.Open(path.Join(downloadedModelPath, blobName))
		if err != nil {
			return fmt.Errorf("error opening blob file: %v", err.Error())
		}
		defer blobFile.Close()

		destinationBlobFile, err := os.Create(parseBlobsDestinationPath(path.Join(blobsFolderPath, blobName)))
		if err != nil {
			return fmt.Errorf("error creating blob file: %v", err.Error())
		}
		defer destinationBlobFile.Close()

		_, err = io.Copy(destinationBlobFile, blobFile)
		if err != nil {
			return fmt.Errorf("error copying blob file: %v", err.Error())
		}

		err = destinationBlobFile.Sync()
		if err != nil {
			return fmt.Errorf("error syncing blob file: %v", err.Error())
		}
	}

	fmt.Println("Model installed successfully")
	return nil
}
