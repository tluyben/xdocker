# xdocker

xdocker is an extended Docker Compose wrapper that provides additional functionality and flexibility for managing Docker environments.

## Features

- **Environment Variable Resolution**: Automatically resolves environment variables in your configuration files.
- **Expression Evaluation**: Supports custom expressions using the FEEL (Friendly Enough Expression Language) engine.
- **Custom Instructions**: Allows defining and using custom instructions through YAML extension files.
- **Version Checking**: Ensures compatibility by checking Docker and Docker Compose versions.
- **Remote Installation**: Supports installation on remote hosts via SSH.
- **Docker System Prune**: Includes a command to clean up Docker resources.

## Installation

1. Clone the repository:

   ```
   git clone https://github.com/yourusername/xdocker.git
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

### Custom Instructions

xdocker supports custom instructions defined in YAML files. Place your custom instruction definitions in the `extensions` directory.

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
  {{if (shouldSkip) then "profiles:\n  - donotstart\n" else ""}}
```

Use in your xdocker-compose.yml:

```yaml
services:
  myservice:
    image: myimage
    skip: true
```

## Environment Variables

xdocker automatically loads environment variables from a `.env` file in the same directory as your xdocker-compose.yml file. You can also use environment variables directly in your compose file:

```yaml
services:
  myservice:
    image: ${SERVICE_IMAGE}:${SERVICE_TAG}
```

## Expressions

xdocker supports FEEL expressions enclosed in double curly braces:

```yaml
services:
  myservice:
    replicas: { { if ($ENV_PROD == "true") then 3 else 1 } }
```

## Version Requirements

- Docker: 20.10.0 or later
- Docker Compose: 2.20.0 or later

xdocker will check these version requirements before executing most commands.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
