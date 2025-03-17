class Gcx < Formula
  desc "A tool for cross-compiling and publishing Go binaries"
  homepage "https://github.com/sxwebdev/gcx"
  url "https://github.com/sxwebdev/gcx/archive/refs/tags/v0.0.1.tar.gz"
  sha256 ""

  depends_on "go" => :build

  def install
    if Hardware::CPU.intel?
      url = "https://github.com/sxwebdev/gcx/releases/download/#{version}/gcx_#{version}_darwin_amd64.tar.gz"
    else
      url = "https://github.com/sxwebdev/gcx/releases/download/#{version}/gcx_#{version}_darwin_arm64.tar.gz"
    end

    system "curl", "-L", url, "-o", "gcx.tar.gz"
    system "tar", "xzf", "gcx.tar.gz"
    bin.install "gcx"
  end

  test do
    system "#{bin}/gcx", "version"
  end
end 