# abp-msx cross-build for all platforms
BINARY = abp-msx
DISTDIR = dist
LDFLAGS = -s -w

.PHONY: all clean linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64

all: linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64
	@echo "Built all platforms in $(DISTDIR)/"

linux-amd64:
	@mkdir -p $(DISTDIR)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(DISTDIR)/$(BINARY)-linux-amd64 .

linux-arm64:
	@mkdir -p $(DISTDIR)
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(DISTDIR)/$(BINARY)-linux-arm64 .

darwin-amd64:
	@mkdir -p $(DISTDIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(DISTDIR)/$(BINARY)-darwin-amd64 .

darwin-arm64:
	@mkdir -p $(DISTDIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(DISTDIR)/$(BINARY)-darwin-arm64 .

windows-amd64:
	@mkdir -p $(DISTDIR)
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(DISTDIR)/$(BINARY)-windows-amd64.exe .

windows-arm64:
	@mkdir -p $(DISTDIR)
	GOOS=windows GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(DISTDIR)/$(BINARY)-windows-arm64.exe .

clean:
	rm -rf $(DISTDIR)
