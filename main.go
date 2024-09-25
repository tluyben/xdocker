package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	feel "github.com/superisaac/FEEL.go"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
)

const installScript = `#!/bin/bash
set -e

sudo sysctl -w fs.inotify.max_user_watches=10000000

export DEBIAN_FRONTEND=noninteractive

sudo apt-add-repository -y ppa:ansible/ansible

# update system 
sudo apt update
sudo apt install -y apt-transport-https ca-certificates curl software-properties-common vim openssh-client lynx jq unzip net-tools apache2-utils curl lynx openssl fail2ban

# create keys
if [ ! -f "/root/.ssh/id_rsa" ]; then 
    sudo ssh-keygen -q -t rsa -N '' -f /root/.ssh/id_rsa <<<y >/dev/null 2>&1
fi

# install docker
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu focal stable"
sudo apt install -y docker-ce
sudo apt install -y ansible

sudo systemctl enable --now docker

# install docker-compose 
sudo curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
sudo cp /usr/local/bin/docker-compose /usr/local/sbin

if ! id "ubuntu" &>/dev/null; then
    sudo useradd -m -s /bin/bash ubuntu
fi

sudo usermod -aG docker ubuntu

# Use sudo to run newgrp, which will exit immediately
sudo -u ubuntu newgrp docker

echo "Installation completed successfully."
`
type XDockerConfig struct {
	Version  string                 `yaml:"version"`
	Services map[string]interface{} `yaml:"services"`
	Extend   string                 `yaml:"extend,omitempty"`
}
type Extension struct {
	Name      string               `yaml:"name"`
	Required  bool                 `yaml:"required"`
	Path      string               `yaml:"path"`
	Arguments map[string]Argument  `yaml:"arguments"`
	Generate  string               `yaml:"generate"`
}

type Argument struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

var extensions map[string]Extension


func main() {
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	upCmd := flag.NewFlagSet("up", flag.ExitOnError)
	downCmd := flag.NewFlagSet("down", flag.ExitOnError)

	// Install command flags
	remoteHosts := installCmd.String("hosts", "", "Comma-separated list of user@host")
	identityFile := installCmd.String("i", "", "Path to identity file")

	// Up command flags
	upDetach := upCmd.Bool("d", false, "Detached mode")
	upKeepOrphans := upCmd.Bool("keep-orphans", false, "Keep containers for services not defined in the compose file")
	upNoBuild := upCmd.Bool("no-build", false, "Don't build images before starting containers")

	// Down command flags
	downKeepOrphans := downCmd.Bool("keep-orphans", false, "Keep containers for services not defined in the compose file")

	// Global flag
	composeFile := flag.String("f", "xdocker-compose.yml", "Path to xdocker compose file")

	if len(os.Args) < 2 {
		fmt.Println("Expected 'install', 'up', or 'down' subcommands")
		os.Exit(1)
	}

	if err := loadExtensions(); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading extensions: %v\n", err)
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "install":
		installCmd.Parse(os.Args[2:])
		err = run("install", *composeFile, *remoteHosts, *identityFile, false, false, false, nil)
	case "up":
		upCmd.Parse(os.Args[2:])
		err = run("up", *composeFile, "", "", *upDetach, !*upKeepOrphans, !*upNoBuild, upCmd.Args())
	case "down":
		downCmd.Parse(os.Args[2:])
		err = run("down", *composeFile, "", "", false, !*downKeepOrphans, false, downCmd.Args())
	default:
		fmt.Println("Expected 'install', 'up', or 'down' subcommands")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func localInstall() {
	cmd := exec.Command("bash", "-c", installScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error executing script: %v\n", err)
		os.Exit(1)
	}
}

func remoteInstall(hosts string, identityFile string) {
	hostList := strings.Split(hosts, ",")

	for _, host := range hostList {
		parts := strings.Split(host, "@")
		if len(parts) != 2 {
			fmt.Printf("Invalid host format: %s\n", host)
			continue
		}

		user := parts[0]
		hostname := parts[1]

		var auth []ssh.AuthMethod
		if identityFile != "" {
			key, err := ioutil.ReadFile(identityFile)
			if err != nil {
				fmt.Printf("Unable to read identity file: %v\n", err)
				continue
			}
			signer, err := ssh.ParsePrivateKey(key)
			if err != nil {
				fmt.Printf("Unable to parse private key: %v\n", err)
				continue
			}
			auth = append(auth, ssh.PublicKeys(signer))
		} else {
			// Try default SSH keys
			home, _ := os.UserHomeDir()
			key, err := ioutil.ReadFile(filepath.Join(home, ".ssh", "id_rsa"))
			if err == nil {
				signer, err := ssh.ParsePrivateKey(key)
				if err == nil {
					auth = append(auth, ssh.PublicKeys(signer))
				}
			}
		}

		// If no authentication method is available, prompt for password
		if len(auth) == 0 {
			fmt.Printf("Enter password for %s: ", host)
			password, _ := ioutil.ReadAll(os.Stdin)
			auth = append(auth, ssh.Password(strings.TrimSpace(string(password))))
		}

		config := &ssh.ClientConfig{
			User: user,
			Auth: auth,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}

		client, err := ssh.Dial("tcp", hostname+":22", config)
		if err != nil {
			fmt.Printf("Failed to dial: %s\n", err)
			continue
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			fmt.Printf("Failed to create session: %s\n", err)
			continue
		}
		defer session.Close()

		err = session.Run(installScript)
		if err != nil {
			fmt.Printf("Failed to run script on %s: %v\n", host, err)
		} else {
			fmt.Printf("Installation completed successfully on %s\n", host)
		}
	}
	
}

func up(composeFile string, detach, removeOrphans, build bool, services []string) {
	dockerComposeFile, err := processXDockerFile(composeFile)
	if err != nil {
		fmt.Printf("Error processing xdocker file: %v\n", err)
		os.Exit(1)
	}

	args := []string{"-f", dockerComposeFile, "up"}
	if detach {
		args = append(args, "-d")
	}
	if removeOrphans {
		args = append(args, "--remove-orphans")
	}
	if build {
		args = append(args, "--build")
	}
	args = append(args, services...)

	err = runDockerCompose(args...)
	if err != nil {
		fmt.Printf("Error running docker-compose up: %v\n", err)
		os.Exit(1)
	}
}

func down(composeFile string, removeOrphans bool, services []string) {
	dockerComposeFile, err := processXDockerFile(composeFile)
	if err != nil {
		fmt.Printf("Error processing xdocker file: %v\n", err)
		os.Exit(1)
	}

	args := []string{"-f", dockerComposeFile, "down"}
	if removeOrphans {
		args = append(args, "--remove-orphans")
	}
	args = append(args, services...)

	err = runDockerCompose(args...)
	if err != nil {
		fmt.Printf("Error running docker-compose down: %v\n", err)
		os.Exit(1)
	}
}

func runDockerCompose(args ...string) error {
	cmd := exec.Command("docker-compose", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		// Check if the error is due to container already existing
		if strings.Contains(err.Error(), "already exists") {
			fmt.Println("Container already exists. Removing and trying again...")
			removeArgs := append([]string{"-f", args[1], "rm", "-f"}, args[len(args)-1])
			removeCmd := exec.Command("docker-compose", removeArgs...)
			removeCmd.Stdout = os.Stdout
			removeCmd.Stderr = os.Stderr
			err = removeCmd.Run()
			if err != nil {
				return fmt.Errorf("error removing existing container: %v", err)
			}
			// Try the original command again
			return runDockerCompose(args...)
		}
		return err
	}
	return nil
}

func processXDockerFile(inputFile string) (string, error) {
	// Load .env file
	envFile := filepath.Join(filepath.Dir(inputFile), ".env")
	err := godotenv.Load(envFile)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("error loading .env file: %v", err)
	}

	config, err := readAndMergeConfigs(inputFile)
	if err != nil {
		return "", fmt.Errorf("error processing xdocker files: %v", err)
	}

	// Resolve all environment variables and expressions in the config
	err = resolveAllEnvVariablesAndExpressions(config)
	if err != nil {
		return "", fmt.Errorf("error resolving environment variables and expressions: %v", err)
	}

	// Process custom instructions here
	err = processCustomInstructions(config)
	if err != nil {
		return "", fmt.Errorf("error processing custom instructions: %v", err)
	}

	outputFile := fmt.Sprintf("docker-compose-%s.yml", filepath.Base(inputFile))
	outputData, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("error generating docker-compose file: %v", err)
	}

	err = ioutil.WriteFile(outputFile, outputData, 0644)
	if err != nil {
		return "", fmt.Errorf("error writing docker-compose file: %v", err)
	}

	return outputFile, nil
}

func resolveAllEnvVariablesAndExpressions(config *XDockerConfig) error {
	for serviceName, serviceConfig := range config.Services {
		service := serviceConfig.(map[interface{}]interface{})
		err := resolveEnvVariablesAndExpressionsInMap(service)
		if err != nil {
			return fmt.Errorf("error in service %s: %v", serviceName, err)
		}
		config.Services[serviceName] = service
	}
	return nil
}

func resolveEnvVariablesAndExpressionsInMap(m map[interface{}]interface{}) error {
	for key, value := range m {
		switch v := value.(type) {
		case string:
			resolved, err := resolveEnvVariablesAndExpressionsInString(v)
			if err != nil {
				return fmt.Errorf("error resolving value for key '%v': %v", key, err)
			}
			m[key] = resolved
		case map[interface{}]interface{}:
			err := resolveEnvVariablesAndExpressionsInMap(v)
			if err != nil {
				return err
			}
		case []interface{}:
			err := resolveEnvVariablesAndExpressionsInSlice(v)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveEnvVariablesAndExpressionsInSlice(s []interface{}) error {
	for i, value := range s {
		switch v := value.(type) {
		case string:
			resolved, err := resolveEnvVariablesAndExpressionsInString(v)
			if err != nil {
				return fmt.Errorf("error resolving value at index %d: %v", i, err)
			}
			s[i] = resolved
		case map[interface{}]interface{}:
			err := resolveEnvVariablesAndExpressionsInMap(v)
			if err != nil {
				return err
			}
		case []interface{}:
			err := resolveEnvVariablesAndExpressionsInSlice(v)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveEnvVariablesAndExpressionsInString(s string) (string, error) {
	// First, resolve environment variables
	reEnv := regexp.MustCompile(`\$(\w+)|\$\{(\w+)\}`)
	var missingVars []string
	s = reEnv.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[1:] // Remove the leading $
		if varName[0] == '{' {
			varName = varName[1 : len(varName)-1] // Remove { and }
		}
		if value, exists := os.LookupEnv(varName); exists {
			return value
		}
		missingVars = append(missingVars, varName)
		return match // Keep original for error reporting
	})

	if len(missingVars) > 0 {
		return "", fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	// Then, evaluate expressions
	reExpr := regexp.MustCompile(`\{\{(.+?)\}\}`)
	return reExpr.ReplaceAllStringFunc(s, func(match string) string {
		expr := match[2 : len(match)-2] // Remove {{ and }}
		res, err := feel.EvalString(expr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Expression evaluation error: %s\n", err)
			return match // Return original if evaluation fails
		}
		return convertToString(res)
	}), nil
}

func convertToString(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case int:
		return strconv.Itoa(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		bytes, err := json.Marshal(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error converting value to string: %v\n", err)
			return fmt.Sprintf("%v", v)
		}
		return string(bytes)
	}
}

// func processCustomInstructions(config *XDockerConfig) error {
// 	for serviceName, serviceConfig := range config.Services {
// 		service := serviceConfig.(map[interface{}]interface{})
		
// 		// Process the 'skip' property
// 		if skip, ok := service["skip"]; ok {
// 			delete(service, "skip")
// 			skipValue := fmt.Sprintf("%v", skip)
// 			if skipValue == "true" || skipValue == "yes" {
// 				service["profiles"] = []string{"donotstart"}
// 			}
// 		}

// 		// Add more custom instruction processing here

// 		config.Services[serviceName] = service
// 	}

// 	return nil
// }
func mergeConfigs(parent, child *XDockerConfig) {
	if child.Version == "" {
		child.Version = parent.Version
	}

	for serviceName, serviceConfig := range parent.Services {
		if _, exists := child.Services[serviceName]; !exists {
			child.Services[serviceName] = serviceConfig
		} else {
			// Merge service configurations
			parentService := serviceConfig.(map[interface{}]interface{})
			childService := child.Services[serviceName].(map[interface{}]interface{})
			for key, value := range parentService {
				if _, exists := childService[key]; !exists {
					childService[key] = value
				}
			}
		}
	}

	// Remove the 'extend' field as it's not valid in docker-compose
	child.Extend = ""
}


func readAndMergeConfigsRecursive(inputFile string, visited map[string]bool) (*XDockerConfig, error) {
	if visited[inputFile] {
		return nil, fmt.Errorf("circular dependency detected in file: %s", inputFile)
	}
	visited[inputFile] = true

	data, err := ioutil.ReadFile(inputFile)
	if err != nil {
		return nil, fmt.Errorf("error reading xdocker file %s: %v", inputFile, err)
	}

	var config XDockerConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing xdocker file %s: %v", inputFile, err)
	}

	if config.Extend != "" {
		extendFile := filepath.Join(filepath.Dir(inputFile), config.Extend)
		parentConfig, err := readAndMergeConfigsRecursive(extendFile, visited)
		if err != nil {
			return nil, err
		}
		mergeConfigs(parentConfig, &config)
	}

	return &config, nil
}
func readAndMergeConfigs(inputFile string) (*XDockerConfig, error) {
	visited := make(map[string]bool)
	return readAndMergeConfigsRecursive(inputFile, visited)
}


// func expandEnvVariables(config *XDockerConfig) {
// 	for serviceName, serviceConfig := range config.Services {
// 		service := serviceConfig.(map[interface{}]interface{})
// 		for key, value := range service {
// 			if strValue, ok := value.(string); ok {
// 				service[key] = os.ExpandEnv(strValue)
// 			}
// 		}
// 		config.Services[serviceName] = service
// 	}
// }

func run(command, composeFile, remoteHosts, identityFile string, detach, removeOrphans, build bool, services []string) error {
	switch command {
	case "install":
		if remoteHosts == "" {
			localInstall()
		} else {
			remoteInstall(remoteHosts, identityFile)
		}
	case "up", "down":
		dockerComposeFile, err := processXDockerFile(composeFile)
		if err != nil {
			return fmt.Errorf("error processing xdocker file: %v", err)
		}

		args := []string{"-f", dockerComposeFile, command}
		if command == "up" {
			if detach {
				args = append(args, "-d")
			}
			if build {
				args = append(args, "--build")
			}
		}
		if removeOrphans {
			args = append(args, "--remove-orphans")
		}
		args = append(args, services...)

		err = runDockerCompose(args...)
		if err != nil {
			return fmt.Errorf("error running docker-compose %s: %v", command, err)
		}
	}

	return nil
}

func loadExtensions() error {
	extensions = make(map[string]Extension)
	extensionsDir := "./extensions"
	files, err := ioutil.ReadDir(extensionsDir)
	if err != nil {
		return fmt.Errorf("error reading extensions directory: %v", err)
	}
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".yml" {
			filePath := filepath.Join(extensionsDir, file.Name())
			data, err := ioutil.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("error reading extension file %s: %v", file.Name(), err)
			}
			var ext Extension
			err = yaml.Unmarshal(data, &ext)
			if err != nil {
				return fmt.Errorf("error parsing extension file %s: %v", file.Name(), err)
			}
			extensions[ext.Name] = ext
		}
	}
	return nil
}

func processCustomInstructions(config *XDockerConfig) error {
	for serviceName, serviceConfig := range config.Services {
		service := serviceConfig.(map[interface{}]interface{})
		
		for extName, ext := range extensions {
			if strings.HasPrefix(ext.Path, "/$service/") {
				key := strings.TrimPrefix(ext.Path, "/$service/")
				if value, ok := service[key]; ok {
					result, err := processExtension(ext, fmt.Sprintf("%v", value))
					if err != nil {
						return fmt.Errorf("error processing extension %s for service %s: %v", extName, serviceName, err)
					}
					delete(service, key)
					if result != "" {
						var resultMap map[string]interface{}
						err = yaml.Unmarshal([]byte(result), &resultMap)
						if err != nil {
							return fmt.Errorf("error parsing extension result for %s: %v", extName, err)
						}
						for k, v := range resultMap {
							service[k] = v
						}
					}
				}
			}
		}
		config.Services[serviceName] = service
	}
	return nil
}
func processExtension(ext Extension, value string) (string, error) {
	var bindStatements []string
	for argName, arg := range ext.Arguments {
		var bindExpr string
		switch arg.Type {
		case "bool":
			bindExpr = fmt.Sprintf("bind(\"%s\", '%s' === \"true\" || '%s' === \"1\" || '%s' === \"yes\");", argName, value, value, value)
		case "int":
			bindExpr = fmt.Sprintf("bind(\"%s\", number('%s'));", argName, value)
		case "float":
			bindExpr = fmt.Sprintf("bind(\"%s\", number('%s'));", argName, value)
		default: // string
			bindExpr = fmt.Sprintf("bind(\"%s\", '%s');", argName, value)
		}
		bindStatements = append(bindStatements, bindExpr)
	}

	fullExpression := strings.Join(bindStatements, "\n") + "\n" + ext.Generate

	result, err := feel.EvalString(fullExpression)
	if err != nil {
		return "", fmt.Errorf("error evaluating extension generate expression: %v", err)
	}

	return fmt.Sprintf("%v", result), nil
}