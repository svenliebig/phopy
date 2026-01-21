# Phopy

A photo copy tool for my personal opinionated needs. This tool is useful for you when you copy photos from your camera to your computer less regulary and you want a program to handle duplicates, the RAW/JPEG juggeling and specific date ranges.

## Install

### Homebrew (macOS/Linux)

```bash
brew install svenliebig/tap/phopy
```

### Manual

Download the latest release from the [GitHub Releases page](https://github.com/svenliebig/phopy/releases) and move it into your PATH.

### Verify Installation

```bash
phopy --help
```

## Features

Phopy takes a directory as input and copies the files from that directory to a target directory with the following base conditions:

- Copy all RAW files
- Copy JPEG files when it does not have a correlated RAW file (case of HDR or other photgraphy where the camera does not create a RAW image)
- The program will skip files that already exists in the target directory, and ask for confirmation to replace them.

## Configuration

You can pass phopy configuration directly into the command, some also support ENV variables if specified.

| Option                  | Description                                                                   | ENV Variable        |
|-------------------------|-------------------------------------------------------------------------------|---------------------|
| `--source` or `-s`      | The source directory to copy from.                                            | PHOPY_SOURCE_DIR    |
| `--target` or `-t`      | The target directory to copy to.                                              | PHOPY_TARGET_DIR    |
| `--dry-run` or `-d`     | Whether to perform a dry run (logging only) of the copy operation.            |                     |
| `--verbose` or `-v`     | Whether to print verbose output.                                              | PHOPY_VERBOSE       |
| `--from` or `-f`        | The start date to copy from when the picture was taken, skip earlier.         | PHOPY_FROM          |
| `--until` or `-u`       | The end date to copy to when the picture was taken, skip later.               | PHOPY_UNTIL         |
| `--override` or `-o`    | Whether to override files that already exist in the target directory.         |                     |

## Usage

```bash
phopy --source /path/to/source --target /path/to/target
```

## Build

```bash
go build ./cmd/phopy
```

The binary will be created in the current directory as `phopy`.

