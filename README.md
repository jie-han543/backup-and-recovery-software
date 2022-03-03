# backup-and-recovery-software

### 软件介绍
操作系统：Linux  
开发语言：Go  
编译环境：Go  
GUI：xml + Gtk库  
基础功能：数据备份、数据还原、数据验证、 GUI界面  
附加功能：压缩解压、打包解包、加密备份、自定义(路径&名称)  

### 实现逻辑
1. 对目录/文件，允许**压缩、打包、加密**同时进行，选用**打包**作为判断依据：  
   不打包时在最外层目录下建立隐藏文件保存元数据，打包时按设定格式将元数据等信息保存在包头。  
   如果**同时勾选**压缩与加密，备份时先压缩后加密，恢复时先解密后解压。  
2. 使用Go语言快速实现进程/线程操作。  
   对界面操作：主线程  
   备份/恢复操作：子线程  
   防止冲突：加锁解锁  
   资源释放：```defer xxx.Close();  // 延迟释放语句，确保程序退出或崩溃时及时释放资源```
![界面展示](https://user-images.githubusercontent.com/57163528/156512243-f6857228-84b4-40c9-aee1-0094dd62a280.png)
### 功能实现
1. 压缩解压：使用 Go 内置 gzi 压缩算法 实现   
   定义 compressGZIP 与 decompressGZIP 函数，整合包调用代码   
2. 加密备份：使用 Go 提供的 rc4 加密包 实现  
   加密器 ```cipher, err = rc4.NewCipher(key[:])```，其中密钥 key 为：程序标识符 appId + “:” + 加密密码 password 的MD5散列值  
3. 打包解包：
   ![image](https://user-images.githubusercontent.com/57163528/156511489-96e0292f-98ef-4e99-a1c6-f00297279a29.png)
4. 数据验证：
   可选择参考新的原始目录/原来的备份保存位置进行验证。  
   ![image](https://user-images.githubusercontent.com/57163528/156511620-84a6324c-c53e-4388-a388-a6d30f8b692a.png)  
   双向对比：参考备份在源中查找 & 参考源在备份中查找。  
   (伪)恢复：不是真的恢复，只是恢复到固定大小缓冲区依次比较。未压缩可先比较文件大小得知是否被修改，若文件大小不变再对文件内容进行比对验证。      



### 软件编译
   提供了 makefile 文件，用于使用 make 构建程序；也可直接安装编译好的程序，文件 backup.desktop 用于建立桌面图标。  

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
