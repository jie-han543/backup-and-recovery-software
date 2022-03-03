# backup-and-recovery-software

1. 安装 go  
   ```
   cd ~    
   wget https://golang.org/dl/go1.17.1.linux-amd64.tar.gz  
   sudo rm -rf /usr/local/go  
   sudo tar -C /usr/local -xzf go1.17.1.linux-amd64.tar.gz  
   mkdir go  
   su  
   echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile  
   exit  
   ```
2. 安装 gotk3 库
   ```
   export GOPROXY=https://goproxy.io,direct
   go get -u github.com/gotk3/gotk3/gtk
   go install github.com/gotk3/gotk3/gtk
   ```
3. 如果不使用包模式
   ```
   mkdir ~/go/src
   mkdir ~/go/src/github.com
   cp -r ~/go/pkg/mod/github.com/gotk3 ~/go/src/github.com/gotk3
   cd ~/go/src/github.com/gotk3
   mv gotk3@v0.6.1 gotk3
   export GO111MODULE=off
   ```
4. 安装开发环境
   ```
   sudo apt-get update -y
   sudo apt-get install libgtk-3-dev libcairo2-dev libglib2.0-dev
   ```
5. 可选的安装界面编辑工具
   ```
   sudo apt-get install -y glade
   ```
6. 创建项目
   ```
   cd ~
   mkdir backup
   cd backup
   ```
7. 构建
   ```
   不使用包模式
   GO111MODULE=off GOOS=linux GOARCH=amd64 go build -ldflags "-s -w"  backup.go
   使用包模式
   GO111MODULE=on GOOS=linux GOARCH=amd64 go build -ldflags "-s -w"  backup.go
   ```
