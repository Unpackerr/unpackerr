# binary name.
NAME=unpacker-poller
# used for plist file name on macOS.
ID=com.github.davidnewhall
GOLANGCI_LINT_ARGS=--enable-all -D gochecknoglobals

all: clean build test
	@echo Finished.

clean:
	@echo "Cleaning Local Build"
	rm -f `echo $${p}|cut -d/ -f3`{,.1,.1.gz}

build:
	@echo "Building Binary"
	go build -ldflags "-w -s"

linux:
	GOOS=linux go build -ldflags "-w -s"

install: man
	@echo "If you get errors, you may need sudo."
	GOBIN=/usr/local/bin go install -ldflags "-w -s" ./...
	mkdir -p /usr/local/etc/$(NAME) /usr/local/share/man/man1
	test -f /usr/local/etc/$(NAME)/up.conf || cp up.conf.example /usr/local/etc/$(NAME)/up.conf
	test -d ~/Library/LaunchAgents && cp startup/launchd/$(ID).$(NAME).plist ~/Library/LaunchAgents || true
	test -d /etc/systemd/system && cp startup/systemd/$(NAME).service /etc/systemd/system || true
	mv *.1.gz /usr/local/share/man/man1

uninstall:
	@echo "If you get errors, you may need sudo."
	test -f ~/Library/LaunchAgents/$(ID).$(NAME).plist && launchctl unload ~/Library/LaunchAgents/$(ID).$(NAME).plist || true
	test -f /etc/systemd/system/$(NAME).service && systemctl stop $(NAME) || true
	rm -rf /usr/local/{etc,bin}/$(NAME) /usr/local/share/man/man1/$(NAME).1.gz
	rm -f ~/Library/LaunchAgents/$(ID).$(NAME).plist
	rm -f /etc/systemd/system/$(NAME).service

# Run code tests and lint.
test: lint
	# Testing.
	go test -race -covermode=atomic ./...
lint:
	# Checking lint.
	#golangci-lint run $(GOLANGCI_LINT_ARGS)

man:
	@echo "Build Man Page(s)"
	script/build_manpages.sh ./

deps:
	@echo "Gathering Vendors"
	dep ensure -update
	dep status
