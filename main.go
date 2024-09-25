package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
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

func main() {
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	remoteHosts := installCmd.String("hosts", "", "Comma-separated list of user@host")
	identityFile := installCmd.String("i", "", "Path to identity file")

	if len(os.Args) < 2 {
		fmt.Println("Expected 'install' subcommand")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "install":
		installCmd.Parse(os.Args[2:])
		if *remoteHosts == "" {
			localInstall()
		} else {
			remoteInstall(*remoteHosts, *identityFile)
		}
	default:
		fmt.Println("Expected 'install' subcommand")
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