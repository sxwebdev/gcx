package build

import (
	"testing"
)

func TestOutputDir(t *testing.T) {
	tests := []struct {
		name              string
		usePlatformSuffix bool
		outDir            string
		artifact          Artifact
		want              string
	}{
		{
			name:              "with platform suffix",
			usePlatformSuffix: true,
			outDir:            "dist",
			artifact:          Artifact{BinaryName: "myapp", Version: "v1.0.0", OS: "linux", Arch: "amd64"},
			want:              "dist/myapp_v1.0.0_linux_amd64",
		},
		{
			name:              "with arm suffix",
			usePlatformSuffix: true,
			outDir:            "dist",
			artifact:          Artifact{BinaryName: "myapp", Version: "v1.0.0", OS: "linux", Arch: "arm", Arm: "7"},
			want:              "dist/myapp_v1.0.0_linux_arm_7",
		},
		{
			name:              "without platform suffix",
			usePlatformSuffix: false,
			outDir:            "dist",
			artifact:          Artifact{BinaryName: "myapp", Version: "v1.0.0", OS: "linux", Arch: "amd64"},
			want:              "dist/myapp_v1.0.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputDir(tt.usePlatformSuffix, tt.outDir, tt.artifact)
			if got != tt.want {
				t.Errorf("outputDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
