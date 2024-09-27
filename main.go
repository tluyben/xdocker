package main

import (
	"bufio"
	"bytes"
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

const dockerInstallScript = `#!/bin/bash
set -e

sudo apt-add-repository -y ppa:ansible/ansible

# update system 
sudo apt update
sudo apt install -y apt-transport-https ca-certificates curl software-properties-common

# install docker
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu focal stable"
sudo apt install -y docker-ce

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

echo "Docker installation completed successfully."
`

const xDockerInstallScript = `#!/bin/bash
set -e

# Install Go 1.20.1
GO_VERSION="1.20.1"
wget https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
rm go${GO_VERSION}.linux-amd64.tar.gz

# Add Go to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile
source /etc/profile

# Install xdocker
git clone https://github.com/tluyben/xdocker.git
cd xdocker
make install

echo "Go and xDocker installation completed successfully."
`
const installScript = `#!/bin/bash
set -e

sudo sysctl -w fs.inotify.max_user_watches=10000000

export DEBIAN_FRONTEND=noninteractive

sudo apt-add-repository -y ppa:ansible/ansible

# update system 
sudo apt update
sudo apt install -y apt-transport-https ca-certificates curl software-properties-common vim openssh-client lynx jq unzip net-tools apache2-utils curl lynx openssl fail2ban make

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

# Install Go 1.20.1
GO_VERSION="1.20.1"
wget https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
rm go${GO_VERSION}.linux-amd64.tar.gz

# Add Go to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile
source /etc/profile

# Install xdocker
git clone https://github.com/tluyben/xdocker.git
cd xdocker
make install

# Install Tailscale
curl -fsSL https://tailscale.com/install.sh | sh

# Configure Tailscale
if [ -n "$TAILSCALE_AUTH_KEY" ]; then
    echo "Tailscale auth key provided. Attempting to authenticate..."
    sudo tailscale up --auth-key="$TAILSCALE_AUTH_KEY"
else
    echo "No Tailscale auth key provided. To authenticate, run:"
    echo "sudo tailscale up"
    echo "Then follow the prompts to authenticate."
fi

echo "Installation completed successfully."
`
type XDockerConfig struct {
	Version  string                 `yaml:"version,omitempty"`
	Services map[string]interface{} `yaml:"services"`
	Networks map[string]interface{} `yaml:"networks,omitempty"`
	Extend   string                 `yaml:"extend,omitempty"`
	Args     string                 `yaml:"args,omitempty"`
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

const (
	defaultGlobalExtensionsDir = "/usr/local/share/xdocker/extensions"
)

var (
	extensionsDir string
)

func main() {
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	upCmd := flag.NewFlagSet("up", flag.ExitOnError)
	downCmd := flag.NewFlagSet("down", flag.ExitOnError)
	psCmd := flag.NewFlagSet("ps", flag.ExitOnError)
	iexecCmd := flag.NewFlagSet("iexec", flag.ExitOnError)
	execCmd := flag.NewFlagSet("exec", flag.ExitOnError)


	// Install command flags
	remoteHosts := installCmd.String("hosts", "", "Comma-separated list of user@host")
	identityFile := installCmd.String("i", "", "Path to identity file")
	onlyDocker := installCmd.Bool("only-docker", false, "Install only Docker")
	onlyXDocker := installCmd.Bool("only-xdocker", false, "Install only Go and xDocker")
	// Add Tailscale auth key flag
	tailscaleAuthKeyFlag := installCmd.String("tailscale-auth-key", "", "Tailscale authentication key (can also be set via TAILSCALE_AUTH_KEY env var)")


	// Up command flags
	upDetach := upCmd.Bool("d", false, "Detached mode")
	upKeepOrphans := upCmd.Bool("keep-orphans", false, "Keep containers for services not defined in the compose file")
	upNoBuild := upCmd.Bool("no-build", false, "Don't build images before starting containers")
	upDry := upCmd.Bool("dry", false, "Only generate the docker-compose file without starting containers")
	upTailscaleIP := upCmd.Bool("tailscale-ip", false, "Use Tailscale IP for exposed ports")
	upLocalhost := upCmd.Bool("localhost", false, "Use localhost for exposed ports")

	// Down command flags
	downKeepOrphans := downCmd.Bool("keep-orphans", false, "Keep containers for services not defined in the compose file")
	downDry := downCmd.Bool("dry", false, "Only generate the docker-compose file without stopping containers")

	// Global flag
	composeFile := flag.String("f", "xdocker-compose.yml", "Path to xdocker compose file")



	// extensionsDir = defaultGlobalExtensionsDir
	flag.StringVar(&extensionsDir, "extension-dir", "", "Custom extensions directory")
	flag.Parse()
	if extensionsDir == "" {
		extensionsDir = defaultGlobalExtensionsDir
	}

	if len(os.Args) < 2 {
		fmt.Println("Expected 'install', 'up', 'down', 'ps', 'iexec', or 'exec' subcommands")
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
		tailscaleAuthKey := *tailscaleAuthKeyFlag
		if tailscaleAuthKey == "" {
			tailscaleAuthKey = os.Getenv("TAILSCALE_AUTH_KEY")
		}
		err = run("install", *composeFile, *remoteHosts, *identityFile, false, false, false, nil, *onlyDocker, *onlyXDocker, false, false, false, tailscaleAuthKey)
	case "up":
		upCmd.Parse(os.Args[2:])
		var config *XDockerConfig
		config, err = readAndMergeConfigs(*composeFile)
		if err != nil {
			fmt.Fprintf(os.Stderr,"error reading xdocker file: %v", err)
			os.Exit(1)
		}

		// Parse additional arguments from the config
		var configArgs []string
		if config.Args != "" {
			configArgs = strings.Fields(config.Args)
			config.Args = "" // remove from the docker-compose file as it's not valid 
		}

		// Merge CLI args with config args
		allArgs := append(configArgs, upCmd.Args()...)

		// Process the merged arguments
		upCmd.Parse(allArgs)

		err = run("up", *composeFile, "", "", *upDetach, !*upKeepOrphans, !*upNoBuild, upCmd.Args(), false, false, *upDry, *upTailscaleIP, *upLocalhost, "")
	case "down":
		downCmd.Parse(os.Args[2:])
		// var config *XDockerConfig
		// config, err = readAndMergeConfigs(*composeFile)
		// if err != nil {
		// 	fmt.Printf("Error reading xdocker file: %v\n", err)
		// 	os.Exit(1)
		// }

		// // Parse additional arguments from the config
		// var configArgs []string
		// if config.Args != "" {
		// 	configArgs = strings.Fields(config.Args)
		// 	config.Args = "" // remove from the docker-compose file as it's not valid
		// }

		// // Merge CLI args with config args
		// allArgs := append(configArgs, downCmd.Args()...)

		// // Process the merged arguments
		// downCmd.Parse(allArgs)

		err = run("down", *composeFile, "", "", false, !*downKeepOrphans, false, downCmd.Args(), false, false, *downDry, false, false, "")
	case "ps":
		psCmd.Parse(os.Args[2:])
		err = runPs(*composeFile)
	case "iexec":
		iexecCmd.Parse(os.Args[2:])
		if iexecCmd.NArg() < 1 {
			fmt.Println("iexec requires a container name or service name")
			os.Exit(1)
		}
		err = runIExec(*composeFile, iexecCmd.Arg(0))
	case "exec":
		execCmd.Parse(os.Args[2:])
		if execCmd.NArg() < 2 {
			fmt.Println("exec requires a container name or service name and a command")
			os.Exit(1)
		}
		err = runExec(*composeFile, execCmd.Arg(0), execCmd.Args()[1:])
	default:
		fmt.Println("Expected 'install', 'up', 'down', 'ps', 'iexec', or 'exec' subcommands")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func localInstall(onlyDocker, onlyXDocker bool, tailscaleAuthKey string) {
	var script string
	if onlyDocker {
		script = dockerInstallScript
	} else if onlyXDocker {
		script = xDockerInstallScript
	} else {
		script = installScript
	}

	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), fmt.Sprintf("TAILSCALE_AUTH_KEY=%s", tailscaleAuthKey))

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error executing script: %v\n", err)
		os.Exit(1)
	}
}

func remoteInstall(hosts string, identityFile string, onlyDocker, onlyXDocker bool, tailscaleAuthKey string) {
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

		var script string
		if onlyDocker {
			script = dockerInstallScript
		} else if onlyXDocker {
			script = xDockerInstallScript
		} else {
			script = installScript
		}

		script = fmt.Sprintf("export TAILSCALE_AUTH_KEY='%s'\n%s", tailscaleAuthKey, script)

		err = session.Run(script)
		if err != nil {
			fmt.Printf("Failed to run script on %s: %v\n", host, err)
		} else {
			fmt.Printf("Installation completed successfully on %s\n", host)
		}
	}
	
}

// func up(composeFile string, detach, removeOrphans, build bool, services []string) {
// 	dockerComposeFile, err := processXDockerFile(composeFile)
// 	if err != nil {
// 		fmt.Printf("Error processing xdocker file: %v\n", err)
// 		os.Exit(1)
// 	}

// 	args := []string{"-f", dockerComposeFile, "up"}
// 	if detach {
// 		args = append(args, "-d")
// 	}
// 	if removeOrphans {
// 		args = append(args, "--remove-orphans")
// 	}
// 	if build {
// 		args = append(args, "--build")
// 	}
// 	args = append(args, services...)

// 	err = runDockerCompose(args...)
// 	if err != nil {
// 		fmt.Printf("Error running docker-compose up: %v\n", err)
// 		os.Exit(1)
// 	}
// }

// func down(composeFile string, removeOrphans bool, services []string) {
// 	dockerComposeFile, err := processXDockerFile(composeFile)
// 	if err != nil {
// 		fmt.Printf("Error processing xdocker file: %v\n", err)
// 		os.Exit(1)
// 	}

// 	args := []string{"-f", dockerComposeFile, "down"}
// 	if removeOrphans {
// 		args = append(args, "--remove-orphans")
// 	}
// 	args = append(args, services...)

// 	err = runDockerCompose(args...)
// 	if err != nil {
// 		fmt.Printf("Error running docker-compose down: %v\n", err)
// 		os.Exit(1)
// 	}
// }

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

func processXDockerFile(inputFile string, tailscaleIP, localhost bool) (string, error) {
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

	config.Version = ""
	config.Args = ""

	if tailscaleIP || localhost {
		err = modifyPortMappings(config, tailscaleIP)
		if err != nil {
			return "", fmt.Errorf("error modifying port mappings: %v", err)
		}
	}

	outputFile := fmt.Sprintf("docker-compose-%s.yml", filepath.Base(inputFile))
	outputData, err := customMarshal(config)
	if err != nil {
		return "", fmt.Errorf("error generating docker-compose file: %v", err)
	}

	err = ioutil.WriteFile(outputFile, outputData, 0644)
	if err != nil {
		return "", fmt.Errorf("error writing docker-compose file: %v", err)
	}

	return outputFile, nil
}

func modifyPortMappings(config *XDockerConfig, useTailscale bool) error {
	var ip string
	var err error

	if useTailscale {
		ip, err = getTailscaleIP()
		if err != nil {
			return fmt.Errorf("error getting Tailscale IP: %v", err)
		}
	} else {
		ip = "127.0.0.1"
	}

	for _, serviceConfig := range config.Services {
		service := serviceConfig.(map[interface{}]interface{})
		if ports, ok := service["ports"].([]interface{}); ok {
			for i, port := range ports {
				portStr := port.(string)
				parts := strings.Split(portStr, ":")
				if len(parts) == 2 {
					ports[i] = fmt.Sprintf("%s:%s:%s", ip, parts[0], parts[1])
				} else if len(parts) == 3 {
					ports[i] = fmt.Sprintf("%s:%s:%s", ip, parts[1], parts[2])
				}
			}
			service["ports"] = ports
		}
	}

	return nil
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
func getTailscaleIP() (string, error) {
	cmd := exec.Command("tailscale", "ip", "--4")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error executing tailscale command: %v", err)
	}
	return strings.TrimSpace(string(output)), nil
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
	if child.Networks == nil {
        child.Networks = make(map[string]interface{})
    }
    for networkName, networkConfig := range parent.Networks {
        if _, exists := child.Networks[networkName]; !exists {
            child.Networks[networkName] = networkConfig
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

func run(command, composeFile, remoteHosts, identityFile string, detach, removeOrphans, build bool, services []string, onlyDocker, onlyXDocker, dry, tailscaleIP, localhost bool, tailscaleAuthKey string) error {


	switch command {
	case "install":
		if remoteHosts == "" {
			localInstall(onlyDocker, onlyXDocker, tailscaleAuthKey)
		} else {
			remoteInstall(remoteHosts, identityFile, onlyDocker, onlyXDocker, tailscaleAuthKey)
		}
	case "up", "down":
		dockerComposeFile, err := processXDockerFile(composeFile, tailscaleIP, localhost)
		if err != nil {
			return fmt.Errorf("error processing xdocker file: %v", err)
		}

		if dry {
			fmt.Printf("Docker Compose file generated: %s\n", dockerComposeFile)
			return nil
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

func customMarshal(in interface{}) ([]byte, error) {
   yamlData, err := yaml.Marshal(in)
   if err != nil {
      return nil, err
   }

   var buf bytes.Buffer
   var indent int
   var inService bool
   var previousLine string

   scanner := bufio.NewScanner(bytes.NewReader(yamlData))
   for scanner.Scan() {
      line := scanner.Text()
      trimmedLine := strings.TrimSpace(line)

      if strings.HasPrefix(trimmedLine, "services:") {
         inService = true
      }

      if inService && strings.HasSuffix(previousLine, ":") && !strings.HasSuffix(trimmedLine, ":") {
         indent = 6
      } else if strings.HasSuffix(trimmedLine, ":") {
         indent = 3 * (strings.Count(line, " ") / 2)
      }

      if trimmedLine == "" {
         if inService && !strings.HasPrefix(previousLine, "services:") {
            buf.WriteString("\n")
         }
         buf.WriteString("\n")
      } else {
         indentedLine := strings.Repeat("   ", indent) + trimmedLine + "\n"
         buf.WriteString(indentedLine)
      }

      previousLine = trimmedLine
   }

   return buf.Bytes(), nil
}

func loadExtensions() error {
	extensions = make(map[string]Extension)
	files, err := ioutil.ReadDir(extensionsDir)
	if err != nil {
		return fmt.Errorf("error reading extensions directory %s: %v", extensionsDir, err)
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
							return fmt.Errorf("error parsing extension result for %s: %v\nResult:\n%s", extName, err, result)
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
			bindExpr = fmt.Sprintf("bind(\"%s\", \"%s\" = \"true\" or \"%s\" = \"1\" or \"%s\" = \"yes\");", argName, value, value, value)
		case "int":
			intValue, err := strconv.Atoi(value)
			if err != nil {
				return "", fmt.Errorf("error converting value to int: %v", err)
			}
			bindExpr = fmt.Sprintf("bind(\"%s\", %d);", argName, intValue)
		case "float":
			floatValue, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return "", fmt.Errorf("error converting value to float: %v", err)
			}
			bindExpr = fmt.Sprintf("bind(\"%s\", %f);", argName, floatValue)
		default: // string
			bindExpr = fmt.Sprintf("bind(\"%s\", \"%s\");", argName, value)
		}
		bindStatements = append(bindStatements, bindExpr)
	}

	// Remove {{}} from the generate expression
	generateExpr := strings.TrimSpace(ext.Generate)
	generateExpr = strings.TrimPrefix(generateExpr, "{{")
	generateExpr = strings.TrimSuffix(generateExpr, "}}")

	fullExpression := strings.Join(bindStatements, "") + "" + generateExpr

	result, err := feel.EvalString(fullExpression)
	if err != nil {
		return "", fmt.Errorf("error evaluating extension generate expression: %v\nFull expression:\n%s", err, fullExpression)
	}

	return fmt.Sprintf("%v", result), nil
}

func runPs(composeFile string) error {
	cmd := exec.Command("docker-compose", "-f", composeFile, "ps")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runIExec(composeFile, containerOrService string) error {
	containerName, err := getContainerName(composeFile, containerOrService)
	if err != nil {
		return err
	}

	shell := "/bin/bash"
	if !shellExists(containerName, shell) {
		shell = "/bin/sh"
	}

	cmd := exec.Command("docker", "exec", "-it", containerName, shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runExec(composeFile, containerOrService string, command []string) error {
	containerName, err := getContainerName(composeFile, containerOrService)
	if err != nil {
		return err
	}

	args := append([]string{"exec", "-t", containerName}, command...)
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getContainerName(composeFile, containerOrService string) (string, error) {
	// First, check if it's a valid container name
	if containerExists(containerOrService) {
		return containerOrService, nil
	}

	// If not, try to get the container name from the service name
	cmd := exec.Command("docker-compose", "-f", composeFile, "ps", "-q", containerOrService)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error getting container name: %v", err)
	}

	containerName := strings.TrimSpace(string(output))
	if containerName == "" {
		return "", fmt.Errorf("no container found for service: %s", containerOrService)
	}

	return containerName, nil
}

func containerExists(containerName string) bool {
	cmd := exec.Command("docker", "inspect", containerName)
	return cmd.Run() == nil
}

func shellExists(containerName, shell string) bool {
	cmd := exec.Command("docker", "exec", containerName, "which", shell)
	return cmd.Run() == nil
}