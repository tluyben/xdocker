# xdocker

xdocker is an extended Docker Compose wrapper that provides additional functionality and flexibility for managing Docker environments.

## Features

- **Environment Variable Resolution**: Automatically resolves environment variables in your configuration files.
- **Expression Evaluation**: Supports custom expressions using Lua for dynamic configuration.
- **Custom Instructions**: Allows defining and using custom instructions through YAML extension files.
- **Version Checking**: Ensures compatibility by checking Docker and Docker Compose versions.
- **Remote Installation**: Supports installation on remote hosts via SSH.
- **Docker System Prune**: Includes a command to clean up Docker resources.
- **Flexible Installation**: Options to install only Docker or only xdocker components.
- **Dry Run**: Ability to generate Docker Compose files without starting containers.
- **IP Address Binding**: Options to bind exposed ports to Tailscale IP or localhost.
- **Default Arguments**: Ability to specify default arguments in the xdocker-compose file.
- **Service Management**: Commands to add, remove, skip, and unskip services.
- **Port Management**: Commands to add, remove, and update port mappings.
- **Volume Management**: Commands to add, remove, and update volume mappings.
- **Config Extension**: Ability to extend and merge multiple configuration files.
- **Tailscale Integration**: Option to use Tailscale IP for exposed ports.
- **Interactive Shell**: Command to open an interactive shell in a container.
- **Command Execution**: Ability to execute commands in containers.

## Installation

1. Clone the repository:

   ```
   git clone https://github.com/tluyben/xdocker.git
   ```

2. Navigate to the xdocker directory:

   ```
   cd xdocker
   ```

3. Build the xdocker binary:

   ```
   go build -o xdocker
   ```

   or

   ```
   make
   ```

4. (Optional) Move the binary to a directory in your PATH for easy access:

   ```
   sudo mv xdocker /usr/local/bin/
   ```

   or

   ```
   make install
   ```

## Usage

### Basic Commands

- **Up**: Start your Docker Compose services

  ```
  xdocker up
  ```

- **Down**: Stop your Docker Compose services

  ```
  xdocker down
  ```

- **Install**: Set up xdocker environment (local or remote)

  ```
  xdocker install
  ```

- **PS**: List containers

  ```
  xdocker ps
  ```

- **Interactive Exec**: Open an interactive shell in a container

  ```
  xdocker iexec <container_or_service>
  ```

- **Exec**: Execute a command in a container
  ```
  xdocker exec <container_or_service> <command>
  ```

### Additional Options

- **Clean**: Run Docker system prune without confirmation

  ```
  xdocker --clean
  ```

- **Custom Compose File**: Specify a custom xdocker-compose file

  ```
  xdocker up -f my-custom-compose.yml
  ```

- **Detached Mode**: Run containers in the background

  ```
  xdocker up -d
  ```

- **Install Only Docker**: Install only Docker components

  ```
  xdocker install --only-docker
  ```

- **Install Only xDocker**: Install only Go and xdocker components

  ```
  xdocker install --only-xdocker
  ```

- **Dry Run**: Generate Docker Compose file without starting containers

  ```
  xdocker up --dry
  ```

- **Use Tailscale IP**: Bind exposed ports to Tailscale IP

  ```
  xdocker up --tailscale-ip
  ```

- **Use Localhost**: Bind exposed ports to localhost

  ```
  xdocker up --localhost
  ```

- **Exclude Services from IP Binding**: Exclude specific services from IP binding

  ```
  xdocker up --exclude service1,service2
  ```

- **Bind Services to Global IP**: Bind specific services to 0.0.0.0

  ```
  xdocker up --global service1,service2
  ```

### Service Management

- **Add Service**: Add a new service to the compose file

  ```
  xdocker add <service_name>
  ```

- **Remove Service**: Remove a service from the compose file

  ```
  xdocker remove <service_name>
  ```

- **Skip Service**: Mark a service to be skipped during deployment

  ```
  xdocker skip <service_name>
  ```

- **Unskip Service**: Remove the skip flag from a service
  ```
  xdocker unskip <service_name>
  ```

### Port Management

- **Add Port**: Add a port mapping to a service

  ```
  xdocker add-port <service_name> <port_mapping>
  ```

- **Remove Port**: Remove a port mapping from a service

  ```
  xdocker remove-port <port>
  ```

- **Update Port**: Update an existing port mapping
  ```
  xdocker update-port <old_port> <new_port>
  ```

### Volume Management

- **Add Volume**: Add a volume mapping to a service

  ```
  xdocker add-volume <service_name> <volume_mapping>
  ```

- **Remove Volume**: Remove a volume mapping from a service

  ```
  xdocker remove-volume <service_name> <volume>
  ```

- **Update Volume**: Update an existing volume mapping
  ```
  xdocker update-volume <service_name> <old_volume> <new_volume>
  ```

### Custom Instructions

xdocker supports custom instructions defined in YAML files. Place your custom instruction definitions in the `extensions` directory. These instructions use Lua for dynamic content generation.

Example `skip.yml`:

```yaml
name: skip
required: false
path: /$service/skip
arguments:
  shouldSkip:
    type: bool
    description: skip this service
    required: true
generate: |
  {{
  if shouldSkip then
    return "profiles:\n  - donotstart\n"
  else
    return ""
  end
  }}
```

Example `openglobal.yml`:

```yaml
name: openglobal
required: false
path: /$service/open-global
arguments:
  globalMapping:
    type: string
    description: "Global mapping in the format domain.com:compose-port:service-port"
    required: true
generate: |
  {{
  for domain, composePort, servicePort in string.gmatch(globalMapping, "(.+):(%d+):(%d+)") do
    if not domain or not composePort or not servicePort then
      return ""
    end
    return string.format("ports:\n - \"127.0.0.1:%s:%s\"\n", composePort, servicePort)
  end
  }}
```

Use in your xdocker-compose.yml:

```yaml
services:
  myservice:
    image: myimage
    skip: true
    open-global: "example.com:8080:80"
```

### Default Arguments

You can specify default arguments in your xdocker-compose.yml file using the `args` property:

```yaml
version: "3"
args: --tailscale-ip -d
services:
  # ... service definitions
```

These arguments will be applied by default when running `xdocker up` or `xdocker down`. You can still override these arguments by specifying them on the command line.

## Environment Variables

xdocker automatically loads environment variables from a `.env` file in the same directory as your xdocker-compose.yml file. You can also use environment variables directly in your compose file:

```yaml
services:
  myservice:
    image: ${SERVICE_IMAGE}:${SERVICE_TAG}
```

## Expressions

xdocker supports Lua expressions enclosed in double curly braces for dynamic configuration:

```yaml
services:
  myservice:
    replicas: {
        {
          if os.getenv("ENV_PROD") == "true" then
          return 3
          else
          return 1
          end,
        },
      }
```

These Lua expressions allow for more complex logic and dynamic configurations based on environment variables or other conditions.

## Config Extension

You can extend and merge multiple configuration files using the `extend` property:

```yaml
extend: base-config.yml
services:
  # ... additional service definitions
```

## Version Requirements

- Docker: 20.10.0 or later
- Docker Compose: 2.20.0 or later

xdocker will check these version requirements before executing most commands.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
