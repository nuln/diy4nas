FROM ubuntu:22.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    bash \
    php-cli \
    curl \
    git \
    wget \
    sudo \
    jq \
    coreutils \
    adduser \
    zsh \
    && rm -rf /var/lib/apt/lists/*

# Create mock unRAID directory structure
RUN mkdir -p /boot/config/plugins && \
    mkdir -p /usr/local/emhttp/plugins && \
    mkdir -p /etc/rc.d && \
    mkdir -p /root

# Copy plugin directories
COPY plugins/easytier /boot/config/plugins/easytier
COPY plugins/homebrew /boot/config/plugins/homebrew
COPY plugins/ohmyzsh /boot/config/plugins/ohmyzsh

# Symlink rc scripts to /etc/rc.d
RUN for p in easytier homebrew ohmyzsh; do \
        ln -sf /boot/config/plugins/$p/rc.$p /etc/rc.d/rc.$p; \
    done

# Symlink event scripts to emhttp
RUN for p in easytier homebrew ohmyzsh; do \
        mkdir -p /usr/local/emhttp/plugins/$p/event; \
        ln -sf /boot/config/plugins/$p/event.disks_mounted /usr/local/emhttp/plugins/$p/event/disks_mounted; \
    done

# Symlink WebUI files to emhttp
RUN for p in easytier homebrew ohmyzsh; do \
        mkdir -p /usr/local/emhttp/plugins/$p; \
        for f in /boot/config/plugins/$p/*.page /boot/config/plugins/$p/api.php /boot/config/plugins/$p/*.js /boot/config/plugins/$p/style.css; do \
            [ -f "$f" ] && ln -sf "$f" /usr/local/emhttp/plugins/$p/; \
        done; \
        if [ -d /boot/config/plugins/$p/images ]; then \
            mkdir -p /usr/local/emhttp/plugins/$p/images; \
            ln -sf /boot/config/plugins/$p/images/* /usr/local/emhttp/plugins/$p/images/ 2>/dev/null || true; \
        fi; \
    done

# Create mock binaries for testing
# easytier-core: sleeps to simulate a daemon; responds to cli queries
RUN printf '#!/bin/bash\n# Mock daemon: stays running until killed\ncase "$1" in\n  node) echo "{\"hostname\":\"$HOSTNAME\",\"peer_id\":12345,\"version\":\"2.6.4\",\"virtual_ipv4\":{\"addr\":167837953},\"proxy_cidrs\":[]}" ;;\n  peer) echo "[]" ;;\n  route) echo "[]" ;;\n  connector) echo "[]" ;;\n  *) sleep 30 ;;\nesac\n' > /usr/bin/easytier-core && \
    printf '#!/bin/bash\necho "{\"hostname\":\"docker-test\",\"peer_id\":12345,\"version\":\"2.6.4\"}"\n' > /usr/bin/easytier-cli && \
    printf '#!/bin/bash\n# Mock brew: creates a brew-like structure\nmkdir -p /home/linuxbrew/.linuxbrew/bin 2>/dev/null || true\necho "mock-brew $@"\nexit 0\n' > /usr/bin/brew && \
    chmod +x /usr/bin/easytier-core /usr/bin/easytier-cli /usr/bin/brew

# Copy tests
COPY plugins/tests/test_integration.sh /test_integration.sh
RUN chmod +x /etc/rc.d/rc.* /test_integration.sh /usr/bin/easytier-core /usr/bin/easytier-cli /usr/bin/brew

CMD ["/test_integration.sh"]
