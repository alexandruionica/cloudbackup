GO_VERSION = "1.12.2"
GLIDE_VERSION = "0.13.1"
GOLANGCI_LINT_VERSION = "1.16.0"

Vagrant.configure("2") do |config|

  config.vm.define "windows2016", autostart: false do |windows|
    windows.vm.box = "windows2016"
      windows.vm.box_check_update = false

      # Settings specific to a windows box
      windows.vm.guest = :windows
      windows.vm.communicator = :winrm
      windows.winrm.username = "vagrant"
      windows.winrm.password = "vagrant"

      windows.ssh.insert_key = false

      windows.vm.synced_folder "../..", "/Users/vagrant/Documents/golang"

      windows.vm.provider "virtualbox" do |vb|
        # Display the VirtualBox GUI when booting the machine
        vb.gui = true

         # Customize the amount of memory on the VM:
         vb.memory = "2048"
      end

      windows.vm.provision "shell", inline: 'setx GOPATH %USERPROFILE%\Documents\golang\ '
  end

  config.vm.define "freebsd11.2", autostart: false do |freebsd|
    freebsd.vm.box = "freebsd/FreeBSD-11.2-STABLE"
    # NFS needs a private network
    freebsd.vm.network "private_network", :ip => "172.28.128.4", :name => 'vboxnet0'

      freebsd.vm.provider "virtualbox" do |vb|
        # Display the VirtualBox GUI when booting the machine
        vb.gui = true
        # Customize the amount of memory on the VM:
        vb.memory = "2048"
      end
      freebsd.vm.guest = :freebsd
      freebsd.ssh.shell = "sh"

      # FreeBSD doesn't support shared folder mounting via VirtualBox so rsync needs to be used(NFS is another option but sqlite doesn't like it so it could lead to funny test output)
      freebsd.vm.synced_folder "../..", "/home/vagrant/Documents/golang", type: "rsync", rsync__exclude: [".git/", "bin/"], rsync__args: ["--verbose", "--archive", "--delete", "-z"]
      # disable default shared folder
      freebsd.vm.synced_folder ".", "/vagrant", disabled: true

      freebsd.vm.provision "shell", inline: "su root -c 'pkg update'"
      freebsd.vm.provision "shell", inline: "pkg install --yes python36 py36-virtualenv py36-pip py36-sqlite3 wget openjdk8-jre bash gcc ca_root_nss git gmake ca_root_nss"
      # TODO - install some kind of Docker Server (unfortunately the old package docker-freebsd is as of now broken and no more available via pkg_install
      # the GO package is a dependency of Docker but otherwise clashes with the custom version we want ...
      freebsd.vm.provision "shell", inline: "(pkg info go && pkg remove --yes --force go) || echo"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/virtualenv || ln -s /usr/local/bin/virtualenv-3.6 /usr/local/bin/virtualenv"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/python3 || ln -s /usr/local/bin/python3.6 /usr/local/bin/python3"
      # setup Docker dependencies
      freebsd.vm.provision "shell", inline: "test -f /usr/local/dockerfs || dd if=/dev/zero of=/usr/local/dockerfs bs=1024K count=3000"
      freebsd.vm.provision "shell", inline: "zpool list zroot || zpool create -f zroot /usr/local/dockerfs"
      freebsd.vm.provision "shell", inline: "zfs list zroot/docker || zfs create -o mountpoint=/usr/docker zroot/docker"
      freebsd.vm.provision "shell", inline: "pw usermod vagrant -G wheel,operator"
      freebsd.vm.provision "shell", inline: "sysrc -f /etc/rc.conf docker_enable='YES'"
      # install GO
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || wget https://dl.google.com/go/go#{GO_VERSION}.freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || tar -xzf go#{GO_VERSION}.freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || mv go /usr/local/go/"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || rm -f go#{GO_VERSION}.freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || ln -s /usr/local/go/bin/go /usr/local/bin/"
      # install GO linter (before GOPATH is set)
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || go get -u github.com/golangci/golangci-lint/cmd/golangci-lint"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || cp /root/go/bin/golangci-lint /usr/local/bin/"
      # install Glide
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/glide || wget https://github.com/Masterminds/glide/releases/download/v#{GLIDE_VERSION}/glide-v#{GLIDE_VERSION}-freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/glide || tar -xzf glide-v#{GLIDE_VERSION}-freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/glide || chmod +x freebsd-amd64/glide"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/glide || cp freebsd-amd64/glide /usr/local/bin/"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/glide || rm -rf glide-v#{GLIDE_VERSION}-freebsd-amd64.tar.gz freebsd-amd64/"
      # enable UTF8 System wide
      freebsd.vm.provision "shell", inline: 'sed -i -e "s/:priority=0:\\\\\/:priority=0::charset=UTF-8::lang=en_US.UTF-8:\\\\\/" /etc/login.conf'
      freebsd.vm.provision "shell", inline: "cap_mkdb /etc/login.conf"

      freebsd.vm.provision "shell", inline: 'grep -q GOPATH /home/vagrant/.cshrc || echo "setenv GOPATH /home/vagrant/Documents/golang/" >> /home/vagrant/.cshrc'

  end


  config.vm.define "ubuntu16.04", autostart: false do |linux|
    linux.vm.box = "ubuntu/xenial64"

      linux.vm.provider "virtualbox" do |vb|
        # Display the VirtualBox GUI when booting the machine
        vb.gui = true
        # Customize the amount of memory on the VM:
        vb.memory = "2048"
      end

        linux.vm.guest = :linux

        linux.vm.synced_folder "../..", "/home/vagrant/Documents/golang"
        # disable default shared folder
        linux.vm.synced_folder ".", "/vagrant", disabled: true

        linux.vm.provision "shell", inline: "sudo apt-get update"
        linux.vm.provision "shell", inline: "sudo DEBIAN_FRONTEND=noninteractive apt-get -y dist-upgrade"
        linux.vm.provision "shell", inline: "sudo apt-get -y install virtualenv python3-virtualenv python3-pip make wget openjdk-8-jre docker.io"
        linux.vm.provision "shell", inline: "sudo usermod -aG docker ubuntu"

        # install GO
        linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || wget https://dl.google.com/go/go#{GO_VERSION}.linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || tar -xzf go#{GO_VERSION}.linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || mv go /usr/local/go/"
        linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || rm -f go#{GO_VERSION}.linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || ln -s /usr/local/go/bin/go /usr/local/bin/"

        # install GO linter
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || wget https://github.com/golangci/golangci-lint/releases/download/v#{GOLANGCI_LINT_VERSION}/golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || tar -xzf golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || cp golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint /usr/local/bin/"
        linux.vm.provision "shell", inline: "rm -rf golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64 golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"

        # Install Glide
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || wget https://github.com/Masterminds/glide/releases/download/v#{GLIDE_VERSION}/glide-v#{GLIDE_VERSION}-linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || tar -xzf glide-v#{GLIDE_VERSION}-linux-amd64.tar.gz"
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || chmod +x linux-amd64/glide"
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || cp linux-amd64/glide /usr/local/bin/"
        linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || rm -rf glide-v#{GLIDE_VERSION}-linux-amd64.tar.gz linux-amd64/"

        linux.vm.provision "shell", inline: 'grep -q GOPATH /home/vagrant/.profile || echo "export GOPATH=/home/vagrant/Documents/golang/" >> /home/vagrant/.profile'
    end


  config.vm.define "ubuntu18.04", autostart: false do |linux|
      linux.vm.box = "ubuntu/bionic64"

        linux.vm.provider "virtualbox" do |vb|
          # Display the VirtualBox GUI when booting the machine
          vb.gui = true
          # Customize the amount of memory on the VM:
          vb.memory = "2048"
        end

          linux.vm.guest = :linux

          linux.vm.synced_folder "../..", "/home/vagrant/Documents/golang"
          # disable default shared folder
          linux.vm.synced_folder ".", "/vagrant", disabled: true

          linux.vm.provision "shell", inline: "sudo apt-get update"
          linux.vm.provision "shell", inline: "sudo DEBIAN_FRONTEND=noninteractive apt-get -y dist-upgrade"
          linux.vm.provision "shell", inline: "sudo apt-get -y install virtualenv python3-virtualenv python3-pip make wget openjdk-8-jre docker.io"
          linux.vm.provision "shell", inline: "sudo usermod -aG docker ubuntu"

          # install GO
          linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || wget https://dl.google.com/go/go#{GO_VERSION}.linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || tar -xzf go#{GO_VERSION}.linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || mv go /usr/local/go/"
          linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || rm -f go#{GO_VERSION}.linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -h /usr/local/bin/go || ln -s /usr/local/go/bin/go /usr/local/bin/"

          # install GO linter
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || wget https://github.com/golangci/golangci-lint/releases/download/v#{GOLANGCI_LINT_VERSION}/golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || tar -xzf golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || cp golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64/golangci-lint /usr/local/bin/"
          linux.vm.provision "shell", inline: "rm -rf golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64 golangci-lint-#{GOLANGCI_LINT_VERSION}-linux-amd64.tar.gz"

          # Install Glide
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || wget https://github.com/Masterminds/glide/releases/download/v#{GLIDE_VERSION}/glide-v#{GLIDE_VERSION}-linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || tar -xzf glide-v#{GLIDE_VERSION}-linux-amd64.tar.gz"
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || chmod +x linux-amd64/glide"
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || cp linux-amd64/glide /usr/local/bin/"
          linux.vm.provision "shell", inline: "test -f /usr/local/bin/glide || rm -rf glide-v#{GLIDE_VERSION}-linux-amd64.tar.gz linux-amd64/"

          linux.vm.provision "shell", inline: 'grep -q GOPATH /home/vagrant/.profile || echo "export GOPATH=/home/vagrant/Documents/golang/" >> /home/vagrant/.profile'
      end
end
