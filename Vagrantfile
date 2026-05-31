GO_VERSION = "1.17.4"
GOLANGCI_LINT_VERSION = "1.43.0"

Vagrant.configure("2") do |config|

  config.vm.define "windows2025", autostart: false do |windows|
    windows.vm.box = "windows2025"
      windows.vm.box_check_update = false

      # Settings specific to a windows box
      windows.vm.guest = :windows
      windows.vm.communicator = :winrm
      windows.winrm.username = "vagrant"
      windows.winrm.password = "vagrant"

      windows.ssh.insert_key = false
#       windows.ssh.username = "vagrant"
#       windows.ssh.password = "vagrant"

      windows.vm.synced_folder "../..", "/Users/vagrant/Documents/golang"

      windows.vm.provider "virtualbox" do |vb|
        # Display the VirtualBox GUI when booting the machine
        vb.gui = true

         # Customize the amount of memory on the VM:
         vb.memory = "4096"
      end
# Known to work if next 3 manually executed
#  net use Z: \\vboxsvr\Users_vagrant_Documents_golang
#  cd Z:\src\cloudbackup
#  make test

      # Workaround for a bug in Go's filepath.EvalSymlinks on Windows when the symlink target is a UNC root - https://github.com/golang/go/issues/42079
    # Point GOPATH at the drive letter we'll mount the share on.
    windows.vm.provision "shell", inline: 'setx /M GOPATH Y:\\'

    # Ensure Y: is mapped at the start of every interactive PowerShell session
    # for the vagrant user. NO /persistent:yes — that's what creates the ghost.
    windows.vm.provision "shell", privileged: true, inline: <<~'PS'
      $share       = '\\vboxsvr\Users_vagrant_Documents_golang'
      $profileDir  = 'C:\Users\vagrant\Documents\WindowsPowerShell'
      $profilePath = Join-Path $profileDir 'profile.ps1'
      New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
      $marker  = '# vagrant-vbox-share-mount'
      # The VBoxSF-Bounce startup task may still be retrying when this
      # profile runs on the first login after boot, so retry the mount
      # until Y: actually appears instead of attempting it just once.
      $snippet = @"
  $marker
  if (-not (Test-Path 'Y:\')) {
    for (`$i = 1; `$i -le 12; `$i++) {
      cmd /c "net use Y: $share" > `$null 2>&1
      if (Test-Path 'Y:\') { break }
      Start-Sleep -Seconds 5
    }
  }
  "@
      # Overwrite rather than append so `vagrant provision` can upgrade the
      # snippet in place when this Vagrantfile changes.
      Set-Content -Path $profilePath -Value $snippet -Encoding ASCII
      # Belt-and-braces: make sure no stale persistent entries linger.
      Remove-Item HKCU:\Network\Y -Recurse -Force -ErrorAction SilentlyContinue
      Remove-Item HKCU:\Network\Z -Recurse -Force -ErrorAction SilentlyContinue
    PS

    # On Windows Server 2025 the VBoxSF kernel driver's UNC redirector comes
    # up wedged after a fresh boot: VBoxControl can list the shared folders
    # but `net use \\vboxsvr\...` fails with system error 64
    # (ERROR_NETNAME_DELETED). Bouncing VBoxSF and restarting VBoxService
    # unwedges it, but only after the system has settled past early-boot —
    # a bounce performed immediately at startup completes without error yet
    # doesn't actually fix the redirector. Drop a self-verifying bounce
    # script on disk and register a startup-triggered task that runs it,
    # retrying with a probe mount until the share is reachable.
    windows.vm.provision "shell", privileged: true, inline: <<~'PS'
      $scriptDir  = 'C:\ProgramData\vagrant'
      $scriptPath = Join-Path $scriptDir 'bounce-vboxsf.ps1'
      New-Item -ItemType Directory -Force -Path $scriptDir | Out-Null

      $bounceScript = @'
$ErrorActionPreference = "Continue"
$probeShare = "\\vboxsvr\Users_vagrant_Documents_golang"
Start-Sleep -Seconds 30
for ($i = 1; $i -le 6; $i++) {
    sc.exe stop  VBoxSF | Out-Null
    sc.exe start VBoxSF | Out-Null
    Restart-Service VBoxService -Force
    Start-Sleep -Seconds 5
    & net use Z: $probeShare 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        & net use Z: /delete 2>&1 | Out-Null
        exit 0
    }
    Start-Sleep -Seconds 10
}
exit 1
'@
      Set-Content -Path $scriptPath -Value $bounceScript -Encoding ASCII

      $taskName  = 'VBoxSF-Bounce'
      $action    = New-ScheduledTaskAction    -Execute 'powershell.exe' `
                                              -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`""
      $trigger   = New-ScheduledTaskTrigger   -AtStartup
      $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -RunLevel Highest
      $settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries `
                                                -DontStopIfGoingOnBatteries `
                                                -ExecutionTimeLimit (New-TimeSpan -Minutes 5)
      Register-ScheduledTask -TaskName $taskName `
                             -Action $action -Trigger $trigger `
                             -Principal $principal -Settings $settings -Force | Out-Null
    PS


  end

  config.vm.define "freebsd12.3", autostart: false do |freebsd|
    freebsd.vm.box = "freebsd/FreeBSD-12.3-STABLE"
    # NFS needs a private network
    freebsd.vm.network "private_network", :ip => "172.28.128.4", :name => 'vboxnet0'

      freebsd.vm.provider "virtualbox" do |vb|
        # Display the VirtualBox GUI when booting the machine
        vb.gui = true
        # Customize the amount of memory on the VM:
        vb.memory = "2048"
      end
      # for this to work, env var VAGRANT_EXPERIMENTAL="disks" needs to be EXPORTED (not just set) before running vagrant
      # increase default disk from 9GB to 20GB . The default depends on the creator of the source "box"
      freebsd.vm.disk :disk, size: "20GB", primary: true

      freebsd.vm.guest = :freebsd
      freebsd.ssh.shell = "sh"

      # FreeBSD doesn't support shared folder mounting via VirtualBox so rsync needs to be used(NFS is another option but sqlite doesn't like it so it could lead to funny test output)
      freebsd.vm.synced_folder "../..", "/home/vagrant/Documents/golang", type: "rsync", rsync__exclude: [".git/", "bin/"], rsync__args: ["--verbose", "--archive", "--delete", "-z"]
      # disable default shared folder
      freebsd.vm.synced_folder ".", "/vagrant", disabled: true

      freebsd.vm.provision "shell", inline: "su root -c 'pkg update'"
      freebsd.vm.provision "shell", inline: "pkg install --yes python38 py38-virtualenv py38-pip py38-sqlite3 wget openjdk8-jre bash gcc ca_root_nss git gmake ca_root_nss rust"
      # TODO - install some kind of Docker Server (unfortunately the old package docker-freebsd is as of now broken and no more available via pkg_install
      # the GO package is a dependency of Docker but otherwise clashes with the custom version we want ...
      freebsd.vm.provision "shell", inline: "(pkg info go && pkg remove --yes --force go) || echo"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/virtualenv || ln -s /usr/local/bin/virtualenv-3.8 /usr/local/bin/virtualenv"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/python3 || ln -s /usr/local/bin/python3.8 /usr/local/bin/python3"
      # setup Docker dependencies
      #freebsd.vm.provision "shell", inline: "test -f /usr/local/dockerfs || dd if=/dev/zero of=/usr/local/dockerfs bs=1024K count=1000"
      #freebsd.vm.provision "shell", inline: "zpool list zroot || zpool create -f zroot /usr/local/dockerfs"
      #freebsd.vm.provision "shell", inline: "zfs list zroot/docker || zfs create -o mountpoint=/usr/docker zroot/docker"
      freebsd.vm.provision "shell", inline: "pw usermod vagrant -G wheel,operator"
      freebsd.vm.provision "shell", inline: "sysrc -f /etc/rc.conf docker_enable='YES'"
      # install GO
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || wget https://dl.google.com/go/go#{GO_VERSION}.freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || tar -xzf go#{GO_VERSION}.freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || mv go /usr/local/go/"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || rm -f go#{GO_VERSION}.freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -h /usr/local/bin/go || ln -s /usr/local/go/bin/go /usr/local/bin/"
      # install GO linter (before GOPATH is set)
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || wget  https://github.com/golangci/golangci-lint/releases/download/v#{GOLANGCI_LINT_VERSION}/golangci-lint-#{GOLANGCI_LINT_VERSION}-freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || tar -xzf golangci-lint-#{GOLANGCI_LINT_VERSION}-freebsd-amd64.tar.gz"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || chmod +x golangci-lint-#{GOLANGCI_LINT_VERSION}-freebsd-amd64/golangci-lint"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || cp golangci-lint-#{GOLANGCI_LINT_VERSION}-freebsd-amd64/golangci-lint /usr/local/bin/"
      freebsd.vm.provision "shell", inline: "test -f /usr/local/bin/golangci-lint || rm -rf golangci-lint-#{GOLANGCI_LINT_VERSION}-freebsd-amd64.tar.gz golangci-lint-#{GOLANGCI_LINT_VERSION}-freebsd-amd64/"


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

          linux.vm.provision "shell", inline: 'grep -q GOPATH /home/vagrant/.profile || echo "export GOPATH=/home/vagrant/Documents/golang/" >> /home/vagrant/.profile'
      end
end
