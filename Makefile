PACKAGES=`find ./cmd -mindepth 1 -maxdepth 1 -type d`
LIBRARYS=./deluge ./starr
NAME=unpacker-poller
ID=com.github.davidnewhall
all: clean build

clean:
	for p in $(PACKAGES); do rm -f `echo $${p}|cut -d/ -f3`{,.1,.1.gz}; done

build:
	for p in $(PACKAGES); do go build -ldflags "-w -s" $${p}; done

linux:
	for p in $(PACKAGES); do GOOS=linux go build -ldflags "-w -s" $${p}; done

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

test: lint
	for p in $(PACKAGES) $(LIBRARYS); do go test -race -covermode=atomic $${p}; done

lint:
	goimports -l $(PACKAGES) $(LIBRARYS)
	gofmt -l $(PACKAGES) $(LIBRARYS)
	errcheck $(PACKAGES) $(LIBRARYS)
	golint $(PACKAGES) $(LIBRARYS)
	go vet $(PACKAGES) $(LIBRARYS)

man:
	script/build_manpages.sh ./

deps:
	dep ensure -update
	dep status
