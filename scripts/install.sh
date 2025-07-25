#!/usr/bin/env bash
set -e

GITHUB_REPO="sxwebdev/gcx"
TARGET_BINARY="gcx"
INSTALL_DIR="/usr/local/bin"

# Determine the operating system
OS=$(uname)
if [ "$OS" == "Darwin" ]; then
  PLATFORM="darwin"
elif [ "$OS" == "Linux" ]; then
  PLATFORM="linux"
else
  echo "Unsupported OS: $OS"
  exit 1
fi

# Determine the architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

echo "Detected platform: $PLATFORM, architecture: $ARCH"

# Fetch the latest release information from GitHub API
API_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
RELEASE_INFO=$(curl --silent "$API_URL")

# Ensure jq is installed
if ! command -v jq &>/dev/null; then
  echo "Error: jq is not installed. Please install jq and try again."
  exit 1
fi

# Find the asset matching the platform and architecture
ASSET_NAME=$(echo "$RELEASE_INFO" | jq -r --arg platform "$PLATFORM" --arg arch "$ARCH" '
  .assets[] | select(.name | test($platform) and test($arch)) | .name
')

if [ -z "$ASSET_NAME" ]; then
  echo "No archive found for platform $PLATFORM and architecture $ARCH"
  exit 1
fi

echo "Found archive: $ASSET_NAME"

# Get the download URL for the asset
ASSET_URL=$(echo "$RELEASE_INFO" | jq -r --arg asset "$ASSET_NAME" '
  .assets[] | select(.name == $asset) | .browser_download_url
')

if [ -z "$ASSET_URL" ]; then
  echo "Failed to obtain download URL."
  exit 1
fi

echo "Downloading binary from $ASSET_URL"
curl -L --silent -o "$ASSET_NAME" "$ASSET_URL"

# Extract the archive
echo "Extracting archive..."
tar -xzf "$ASSET_NAME" || {
  echo "Failed to extract archive."
  exit 1
}

# Remove the downloaded archive after extraction
echo "Removing archive $ASSET_NAME..."
rm "$ASSET_NAME"

# Find the extracted binary file recursively in subdirectories
EXTRACTED_BINARY=$(find . -type f -name "${TARGET_BINARY}" ! -name "*.tar.gz" | head -n 1)

if [ -z "$EXTRACTED_BINARY" ]; then
  echo "Error: No binary file found after extraction."
  exit 1
fi

echo "Extracted binary: $EXTRACTED_BINARY"

# Ensure the extracted file is executable
if [ ! -x "$EXTRACTED_BINARY" ]; then
  echo "Error: Extracted file is not executable."
  exit 1
fi

# Check install directory
if [ ! -d "$INSTALL_DIR" ]; then
  echo "Directory $INSTALL_DIR does not exist."
  exit 1
fi

# Copy the binary to a temporary location for installation
cp "$EXTRACTED_BINARY" "$TARGET_BINARY"

if [ -f "$INSTALL_DIR/$TARGET_BINARY" ]; then
  echo "Removing existing binary from $INSTALL_DIR..."
  sudo rm "$INSTALL_DIR/$TARGET_BINARY"
fi

echo "Moving binary to $INSTALL_DIR..."
sudo mv "$TARGET_BINARY" "$INSTALL_DIR/$TARGET_BINARY"

echo "Making the binary executable..."
sudo chmod +x "$INSTALL_DIR/$TARGET_BINARY"

# Clean up extracted files
echo "Cleaning up extracted files..."
EXTRACTED_DIR=$(dirname "$EXTRACTED_BINARY")
if [ "$EXTRACTED_DIR" != "." ]; then
  rm -rf "$EXTRACTED_DIR"
fi

echo "Installation complete. You can now run $TARGET_BINARY from the terminal."
