#!/usr/bin/env bash
set -e


GITHUB_REPO="sxwebdev/gcx"
TARGET_BINARY="gcx"

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

echo "Downloading binary..."
curl -L --silent -o "$ASSET_NAME" "$ASSET_URL"

# Extract the archive. We assume it contains a single file named like gcx_darwin_amd64.
echo "Extracting archive..."
tar -xzf "$ASSET_NAME"

# Find the extracted binary file (assuming only one file is extracted)
EXTRACTED_BINARY=$(find . -maxdepth 1 -type f -name "gcx_*" | head -n 1)

if [ -z "$EXTRACTED_BINARY" ]; then
    echo "Error: No binary file found after extraction."
    exit 1
fi

echo "Renaming $EXTRACTED_BINARY to $TARGET_BINARY..."
mv "$EXTRACTED_BINARY" "$TARGET_BINARY"

# Directory for installing executables
INSTALL_DIR="/usr/local/bin"

echo "Moving binary to $INSTALL_DIR..."
# Use sudo if necessary
sudo mv "$TARGET_BINARY" "$INSTALL_DIR/$TARGET_BINARY"

echo "Making the binary executable..."
sudo chmod +x "$INSTALL_DIR/$TARGET_BINARY"

# Clean up the downloaded archive
rm "$ASSET_NAME"

echo "Installation complete. You can now run $TARGET_BINARY from the terminal."
