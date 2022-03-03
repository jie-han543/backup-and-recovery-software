package main

import (
    "github.com/gotk3/gotk3/glib"
    "github.com/gotk3/gotk3/gtk"
    "log"
    "os"
    "strings"
    "io"
    "io/ioutil"
    "path"
    "path/filepath"
    "time"
    "bytes"
    "crypto/rc4"
    "crypto/md5"
    "compress/gzip"
    "encoding/binary"
    "sync"
    "sort"
)

// 程序标识符
const appId string = "edu.hanjie.backup";

// 取消备份
var muBackup sync.RWMutex;
var cancelBackup bool;

// 取消恢复
var muRestore sync.RWMutex;
var cancelRestore bool;

// 取消验证
var muVerify sync.RWMutex;
var verifing bool;
var cancelVerify bool;

/**
 * 主程序
 */
func main() {
    app, err := gtk.ApplicationNew(appId, glib.APPLICATION_FLAGS_NONE);
    if err != nil { log.Fatal("创建程序错误", err); }
    app.Connect("activate", func () {
        if builder, err := gtk.BuilderNewFromFile("backup.ui"); err != nil {
            log.Fatal("创建主窗口错误", err);
        } else {
            // 主窗口
            winObj, _ := builder.GetObject("window");
            window := winObj.(*gtk.Window);

            /* 备份部分 --------------------------------------------------------------------------------------------- */

            // 备份源目录输入框
            edtBackupSourceDirObj, _ := builder.GetObject("backupSourceDir");
            edtBackupSourceDir := edtBackupSourceDirObj.(*gtk.Entry);

            // 备份源目录选择按钮
            btnSelectBackupSourceObj, _ := builder.GetObject("selectSourceDir");
            btnSelectBackupSource := btnSelectBackupSourceObj.(*gtk.Button);
            btnSelectBackupSource.Connect("clicked", func () {
                selectFolder("选择备份目录", window, edtBackupSourceDir, false);
            });

            // 备份保存目录输入框
            edtBackupTargetDirObj, _ := builder.GetObject("backupTargetDir");
            edtBackupTargetDir := edtBackupTargetDirObj.(*gtk.Entry);

            // 备份保存目录选择按钮
            btnSelectBackupTargetObj, _ := builder.GetObject("selectTargetDir");
            btnSelectBackupTarget := btnSelectBackupTargetObj.(*gtk.Button);
            btnSelectBackupTarget.Connect("clicked", func () {
                selectFolder("选择备份保存目录", window, edtBackupTargetDir, false);
            })

            // 备份名称输入框
            edtBackupTargetNameObj, _ := builder.GetObject("backupTargetName");
            edtBackupTargetName := edtBackupTargetNameObj.(*gtk.Entry);

            // 压缩勾选框
            chkBackupCompressedObj, _ := builder.GetObject("backupCompressed");
            chkBackupCompressed := chkBackupCompressedObj.(*gtk.CheckButton);

            // 打包勾选框
            chkBackupPackedObj, _ := builder.GetObject("backupPacked");
            chkBackupPacked := chkBackupPackedObj.(*gtk.CheckButton);

            // 密码标签
            lblBackupPasswordObj, _ := builder.GetObject("passwordLabel");
            lblBackupPassword := lblBackupPasswordObj.(*gtk.Label);
            lblBackupPassword.SetSensitive(false);

            // 密码输入框
            edtBackupPasswordObj, _ := builder.GetObject("backupPassword");
            edtBackupPassword := edtBackupPasswordObj.(*gtk.Entry);
            edtBackupPassword.SetSensitive(false);

            // 加密勾选框
            chkBackupEncryptedObj, _ := builder.GetObject("backupEncrypted");
            chkBackupEncrypted := chkBackupEncryptedObj.(*gtk.CheckButton);
            chkBackupEncrypted.SetActive(false);
            chkBackupEncrypted.Connect("toggled", func () {
                encrypt := chkBackupEncrypted.GetActive();
                lblBackupPassword.SetSensitive(encrypt);
                edtBackupPassword.SetSensitive(encrypt);
            });

            // 备份进度条
            prgBackupObj, _ := builder.GetObject("backupProgressBar");
            prgBackup := prgBackupObj.(*gtk.ProgressBar);

            // 备份取消按钮
            btnBackupCancelObj, _ := builder.GetObject("cancelBackupButton");
            btnBackupCancel := btnBackupCancelObj.(*gtk.Button);
            btnBackupCancel.SetSensitive(false);
            btnBackupCancel.Connect("clicked", func () {
                muBackup.Lock();
                defer muBackup.Unlock();
                cancelBackup = true;
            });

            // 备份按钮
            btnBackupObj, _ := builder.GetObject("backupButton");
            btnBackup := btnBackupObj.(*gtk.Button);
            btnBackup.Connect("clicked", func () {
                source, err := edtBackupSourceDir.GetText();
                source = strings.Trim(source, "\t\n\r ");
                if err != nil || source == "" {
                    displayMessage(window, "必须指定备份源目录。", gtk.MESSAGE_WARNING);
                    return;
                }
                if !isFolder(source) {
                    displayMessage(window, "备份源目录必须存在且为目录。", gtk.MESSAGE_WARNING);
                    return;
                }
                target, err := edtBackupTargetDir.GetText();
                target = strings.Trim(target, "\t\n\r ");
                if err != nil || target == "" {
                    displayMessage(window, "必须指定备份保存目录。", gtk.MESSAGE_WARNING)
                    return;
                }
                if !isFolder(target) {
                    displayMessage(window, "备份保存目录必须存在且为目录。", gtk.MESSAGE_WARNING);
                    return;
                }
                packed := chkBackupPacked.GetActive();
                name, err := edtBackupTargetName.GetText();
                name = strings.Trim(name, "\t\n\r ");
                if packed {
                    if err != nil || name == "" {
                        displayMessage(window, "必须指定备份文件名。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    target = path.Join(target, name);
                } else {
                    if isEmptyFolder(target) {
                        if err == nil && name != "" {
                            target = path.Join(target, name);
                        }
                    } else {
                        if err != nil || name == "" {
                            displayMessage(window, "必须指定备份文件夹名称。", gtk.MESSAGE_WARNING);
                            return;
                        }
                        target = path.Join(target, name);
                    }
                }
                if isFolder(target) || isFile(target) {
                    displayMessage(window, "备份文件或文件夹已存在。", gtk.MESSAGE_WARNING);
                    return;
                }
                if strings.HasSuffix(source, string(os.PathSeparator)) {
                    source = source[0:len(source) - 1];
                }
                if source == target || strings.HasPrefix(target, source + string(os.PathSeparator)) {
                    displayMessage(window, "备份不能保存于源路径下。", gtk.MESSAGE_WARNING);
                    return;
                }
                encrypted := chkBackupEncrypted.GetActive();
                password, err := edtBackupPassword.GetText();
                password = strings.Trim(password, "\t\n\r ");
                if err != nil || (password == "" && encrypted) {
                    displayMessage(window, "加密备份时必须指定密码。", gtk.MESSAGE_WARNING)
                    return;
                }
                edtBackupSourceDir.SetSensitive(false);
                btnSelectBackupSource.SetSensitive(false);
                edtBackupTargetDir.SetSensitive(false);
                btnSelectBackupTarget.SetSensitive(false);
                edtBackupTargetName.SetSensitive(false);
                chkBackupCompressed.SetSensitive(false);
                chkBackupPacked.SetSensitive(false);
                chkBackupEncrypted.SetSensitive(false);
                lblBackupPassword.SetSensitive(false);
                edtBackupPassword.SetSensitive(false);
                btnBackup.SetSensitive(false);
                btnBackupCancel.SetSensitive(true);
                prgBackup.SetVisible(true);
                prgBackup.SetFraction(0);
                muBackup.Lock();
                cancelBackup = false;
                muBackup.Unlock();
                go func () {
                    canceled, err := backup(source, target, chkBackupCompressed.GetActive(), packed,
                                            encrypted, password, window, prgBackup);
                    glib.IdleAdd(func () {
                        if canceled {
                            displayMessage(window, "取消备份操作。", gtk.MESSAGE_WARNING);
                        } else if err != nil {
                            displayMessage(window, "备份出错：" + err.Error(), gtk.MESSAGE_ERROR);
                        } else {
                            prgBackup.SetFraction(1);
                            displayMessage(window, "备份完成。", gtk.MESSAGE_INFO);
                        }
                        edtBackupSourceDir.SetSensitive(true);
                        btnSelectBackupSource.SetSensitive(true);
                        edtBackupTargetDir.SetSensitive(true);
                        btnSelectBackupTarget.SetSensitive(true);
                        edtBackupTargetName.SetSensitive(true);
                        chkBackupCompressed.SetSensitive(true);
                        chkBackupPacked.SetSensitive(true);
                        chkBackupEncrypted.SetSensitive(true);
                        encrypt := chkBackupEncrypted.GetActive();
                        lblBackupPassword.SetSensitive(encrypt);
                        edtBackupPassword.SetSensitive(encrypt);
                        btnBackup.SetSensitive(true);
                        btnBackupCancel.SetSensitive(false);
                        prgBackup.SetVisible(false);
                        prgBackup.SetFraction(0);
                        /*
                        edtBackupSourceDir.SetText("");
                        edtBackupTargetDir.SetText("");
                        edtBackuptargetName.SetText("");
                        edtBackupPassword.SetText("");
                        chkBackupCompressed.SetActive(false);
                        chkBackupPacked.SetActive(false);
                        chkBackupEncrypted.SetActive(false);
                        lblBackupPassword.SetSensitive(false);
                        edtBackupPassword.SetSensitive(false);
                        */
                    });
                }();
            });

            /* 恢复部分 --------------------------------------------------------------------------------------------- */

            // 待恢复备份输入框
            edtRestoreBackupObj, _ := builder.GetObject("restoreBackup");
            edtRestoreBackup := edtRestoreBackupObj.(*gtk.Entry);

            // 待恢复备份选择按钮
            btnSelectRestoreBackupObj, _ := builder.GetObject("selectRestoreBackup");
            btnSelectRestoreBackup := btnSelectRestoreBackupObj.(*gtk.Button);
            btnSelectRestoreBackup.Connect("clicked", func () {
                selectFolder("选择备份", window, edtRestoreBackup, true);
            })

            // 恢复至目录标签
            lblRestoreDirLabelObj, _ := builder.GetObject("restoreDirLabel");
            lblRestoreDirLabel := lblRestoreDirLabelObj.(*gtk.Label);
            lblRestoreDirLabel.SetSensitive(false);

            // 恢复至目录输入框
            edtRestoreTargetDirObj, _ := builder.GetObject("restoreTargetDir");
            edtRestoreTargetDir := edtRestoreTargetDirObj.(*gtk.Entry);
            edtRestoreTargetDir.SetSensitive(false);

            // 恢复至目录选择按钮
            btnSelectRestoreDirObj, _ := builder.GetObject("selectRestoreDir");
            btnSelectRestoreDir := btnSelectRestoreDirObj.(*gtk.Button);
            btnSelectRestoreDir.SetSensitive(false);
            btnSelectRestoreDir.Connect("clicked", func () {
                selectFolder("选择恢复目录", window, edtRestoreTargetDir, false);
            });

            // 恢复至原始位置勾选框
            chkRestoreToSourceObj, _ := builder.GetObject("restoreToSource");
            chkRestoreToSource := chkRestoreToSourceObj.(*gtk.CheckButton);
            chkRestoreToSource.SetActive(true);
            chkRestoreToSource.Connect("toggled", func () {
                restoreToDir := !chkRestoreToSource.GetActive();
                edtRestoreTargetDir.SetSensitive(restoreToDir);
                btnSelectRestoreDir.SetSensitive(restoreToDir);
                lblRestoreDirLabel.SetSensitive(restoreToDir);
            });

            // 恢复进度条
            prgRestoreObj, _ := builder.GetObject("restoreProgressBar");
            prgRestore := prgRestoreObj.(*gtk.ProgressBar);

            // 备份取消按钮
            btnRestoreCancelObj, _ := builder.GetObject("cancelRestoreButton");
            btnRestoreCancel := btnRestoreCancelObj.(*gtk.Button);
            btnRestoreCancel.SetSensitive(false);
            btnRestoreCancel.Connect("clicked", func () {
                muRestore.Lock();
                defer muRestore.Unlock();
                cancelRestore = true;
            });

            // 恢复按钮
            btnRestoreObj, _ := builder.GetObject("restoreButton");
            btnRestore := btnRestoreObj.(*gtk.Button);
            btnRestore.Connect("clicked", func() {
                backup, err := edtRestoreBackup.GetText();
                backup = strings.Trim(backup, "\t\n\r ");
                if err != nil || backup == "" {
                    displayMessage(window, "必须指定备份文件或目录。", gtk.MESSAGE_WARNING);
                    return;
                }
                if isFolder(backup) {
                    if !isFile(path.Join(backup, "._backup.meta")) {
                        displayMessage(window, "备份目录不是有效的备份。", gtk.MESSAGE_WARNING);
                        return;
                    }
                } else if !isFile(backup) {
                    displayMessage(window, "备份文件不存在。", gtk.MESSAGE_WARNING);
                    return;
                }
                origina := chkRestoreToSource.GetActive();
                target := "";
                if !origina {
                    target, err = edtRestoreTargetDir.GetText();
                    target = strings.Trim(target, "\t\n\r ");
                    if err != nil || target == "" {
                        displayMessage(window, "必须指定恢复目标目录。", gtk.MESSAGE_WARNING)
                        return;
                    }
                    if !isFolder(target) {
                        displayMessage(window, "恢复目标目录必须存在且为目录。", gtk.MESSAGE_WARNING);
                        return;
                    }
                }
                encrypted := false;
                compressed := false;
                originaDir := "";
                totalFiles := 0;
                metaData := []byte{};
                fileOffset := 0;
                if isFolder(backup) {
                    metaData, err = ioutil.ReadFile(path.Join(backup, "._backup.meta"));
                    if err != nil {
                        displayMessage(window, "读取元数据文件出错。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    if bytes.Compare(metaData[0:len(appId) + 1], []byte(appId + "\n")) != 0 {
                        displayMessage(window, "选择目录不是备份文件夹。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    encrypted = bytes.Compare(metaData[len(appId) + 1:len(appId) + 9], []byte("source: ")) != 0;
                    if encrypted {
                        metaData = metaData[len(appId) + 1:];
                    } else {
                        originaDir = string(bytes.Split(metaData[len(appId) + 9:], []byte("\n"))[0]);
                        compressed = strings.HasSuffix(string(metaData), "\ncompressed: true\n");
                    }
                } else {
                    file, err := os.Open(backup);
                    if err != nil {
                        displayMessage(window, "打开备份文件出错。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    defer file.Close();
                    buff := make([]byte, 65536);
                    size, err := file.Read(buff);
                    if err != nil || size < len(appId) + 55 {
                        displayMessage(window, "打开备份文件出错。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    if bytes.Compare(buff[0:len(appId) + 1], []byte(appId + "\n")) != 0 {
                        displayMessage(window, "选择文件不是备份文件。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    flag := buff[len(appId) + 1];
                    compressed = (flag & 2) == 2;
                    encrypted = (flag & 1) == 1;
                    totalFiles = int(binary.BigEndian.Uint32(buff[len(appId) + 2:len(appId) + 6]));
                    metaLen := int(binary.BigEndian.Uint64(buff[len(appId) + 6:len(appId) + 14]));
                    metaData = buff[len(appId) + 14:len(appId) + 14 + metaLen];
                    if bytes.Compare(metaData[:8], []byte("source: ")) == 0 {
                        originaDir = string(bytes.Split(metaData[8:], []byte("\n"))[0]);
                    }
                    file.Close();
                    fileOffset = len(appId) + 14 + metaLen;
                }
                key := [16]byte{};
                if encrypted {
                    response := 0;
                    dlgPassword, err := createPasswordDialog(window, metaData, &response, &key, &compressed,
                        &originaDir);
                    if err != nil {
                        displayMessage(window, "创建密码输入对话框失败。", gtk.MESSAGE_ERROR);
                        return;
                    }
                    dlgPassword.Run();
                    dlgPassword.Destroy();
                    switch response {
                        case 0:
                            break;
                        case 1:
                            displayMessage(window, "密码错误", gtk.MESSAGE_WARNING);
                            return;
                        case 2:
                            return;
                        case 3:
                            displayMessage(window, "获取密码出错", gtk.MESSAGE_ERROR);
                            return;
                        case 4:
                            displayMessage(window, "创建解密器出错", gtk.MESSAGE_ERROR);
                            return;
                    }
                }
                if origina {
                    target = originaDir;
                }
                edtRestoreBackup.SetSensitive(false);
                btnSelectRestoreBackup.SetSensitive(false);
                chkRestoreToSource.SetSensitive(false);
                lblRestoreDirLabel.SetSensitive(false);
                edtRestoreTargetDir.SetSensitive(false);
                btnSelectRestoreDir.SetSensitive(false);
                btnRestore.SetSensitive(false);
                btnRestoreCancel.SetSensitive(true);
                prgRestore.SetVisible(true);
                prgRestore.SetFraction(0);
                muRestore.Lock();
                cancelRestore = false;
                muRestore.Unlock();
                go func () {
                    canceled, err := restore(backup, target, compressed, encrypted, key, prgRestore,
                        totalFiles, fileOffset);
                    glib.IdleAdd(func() {
                        if canceled {
                            displayMessage(window, "取消恢复操作。", gtk.MESSAGE_WARNING);
                        } else if err != nil {
                            displayMessage(window, "恢复出错：" + err.Error(), gtk.MESSAGE_ERROR);
                        } else {
                            prgRestore.SetFraction(1);
                            displayMessage(window, "恢复完成。", gtk.MESSAGE_INFO);
                        }
                        edtRestoreBackup.SetSensitive(true);
                        btnSelectRestoreBackup.SetSensitive(true);
                        chkRestoreToSource.SetSensitive(true);
                        restoreToDir := !chkRestoreToSource.GetActive();
                        edtRestoreTargetDir.SetSensitive(restoreToDir);
                        btnSelectRestoreDir.SetSensitive(restoreToDir);
                        lblRestoreDirLabel.SetSensitive(restoreToDir);
                        btnRestore.SetSensitive(true);
                        btnRestoreCancel.SetSensitive(false);
                        prgRestore.SetVisible(false);
                        prgRestore.SetFraction(0);
                        /*
                        edtRestoreBackup.SetText("");
                        chkRestoreToSource.SetActive(true);
                        edtRestoreTargetDir.SetText("");
                        edtRestoreTargetDir.SetSensitive(false);
                        btnSelectRestoreDir.SetSensitive(false);
                        lblRestoreDirLabel.SetSensitive(false);
                        */
                    });
                }();
            });

            /* 验证部分 --------------------------------------------------------------------------------------------- */

            // 待验证备份输入框
            edtVerifyBackupObj, _ := builder.GetObject("verifyBackup");
            edtVerifyBackup := edtVerifyBackupObj.(*gtk.Entry);

            // 待验证备份选择按钮
            btnSelectVerifyBackupObj, _ := builder.GetObject("selectVerifyBackup");
            btnSelectVerifyBackup := btnSelectVerifyBackupObj.(*gtk.Button);
            btnSelectVerifyBackup.Connect("clicked", func () {
                selectFolder("选择备份", window, edtVerifyBackup, true);
            })

            // 定制原始路径标签
            lblVerifyDirLabelObj, _ := builder.GetObject("verifyDirLabel");
            lblVerifyDirLabel := lblVerifyDirLabelObj.(*gtk.Label);
            lblVerifyDirLabel.SetSensitive(false);

            // 定制原始路径输入框
            edtVerifyOriginaDirObj, _ := builder.GetObject("verifyOriginaDir");
            edtVerifyOriginaDir := edtVerifyOriginaDirObj.(*gtk.Entry);
            edtVerifyOriginaDir.SetSensitive(false);

            // 定制原始路径选择按钮
            btnSelectVerifyDirObj, _ := builder.GetObject("selectVerifyDir");
            btnSelectVerifyDir := btnSelectVerifyDirObj.(*gtk.Button);
            btnSelectVerifyDir.SetSensitive(false);
            btnSelectVerifyDir.Connect("clicked", func () {
                selectFolder("选择恢复目录", window, edtVerifyOriginaDir, false);
            });

            // 定制原始路径勾选框
            chkVerifyNewSourceObj, _ := builder.GetObject("verifyUseNewSource");
            chkVerifyNewSource := chkVerifyNewSourceObj.(*gtk.CheckButton);
            chkVerifyNewSource.SetActive(false);
            chkVerifyNewSource.Connect("toggled", func () {
                verifyNewSource := chkVerifyNewSource.GetActive();
                edtVerifyOriginaDir.SetSensitive(verifyNewSource);
                btnSelectVerifyDir.SetSensitive(verifyNewSource);
                lblVerifyDirLabel.SetSensitive(verifyNewSource);
            });

            // 验证按钮
            btnVerifyObj, _ := builder.GetObject("verifyButton");
            btnVerify := btnVerifyObj.(*gtk.Button);
            btnVerify.Connect("clicked", func() {
                backup, err := edtVerifyBackup.GetText();
                backup = strings.Trim(backup, "\t\n\r ");
                if err != nil || backup == "" {
                    displayMessage(window, "必须指定备份文件或目录。", gtk.MESSAGE_WARNING);
                    return;
                }
                if isFolder(backup) {
                    if !isFile(path.Join(backup, "._backup.meta")) {
                        displayMessage(window, "备份目录不是有效的备份。", gtk.MESSAGE_WARNING);
                        return;
                    }
                } else if !isFile(backup) {
                    displayMessage(window, "备份文件不存在。", gtk.MESSAGE_WARNING);
                    return;
                }
                newSource := chkVerifyNewSource.GetActive();
                source := "";
                if newSource {
                    source, err = edtVerifyOriginaDir.GetText();
                    source = strings.Trim(source, "\t\n\r ");
                    if err != nil || source == "" {
                        displayMessage(window, "必须指定新的原始备份目录。", gtk.MESSAGE_WARNING)
                        return;
                    }
                    if !isFolder(source) {
                        displayMessage(window, "原始备份目录必须存在且为目录。", gtk.MESSAGE_WARNING);
                        return;
                    }
                }
                encrypted := false;
                compressed := false;
                originaDir := "";
                totalFiles := 0;
                metaData := []byte{};
                fileOffset := 0;
                if isFolder(backup) {
                    metaData, err = ioutil.ReadFile(path.Join(backup, "._backup.meta"));
                    if err != nil {
                        displayMessage(window, "读取元数据文件出错。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    if bytes.Compare(metaData[0:len(appId) + 1], []byte(appId + "\n")) != 0 {
                        displayMessage(window, "选择目录不是备份文件夹。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    encrypted = bytes.Compare(metaData[len(appId) + 1:len(appId) + 9], []byte("source: ")) != 0;
                    if encrypted {
                        metaData = metaData[len(appId) + 1:];
                    } else {
                        originaDir = string(bytes.Split(metaData[len(appId) + 9:], []byte("\n"))[0]);
                        compressed = strings.HasSuffix(string(metaData), "\ncompressed: true\n");
                    }
                } else {
                    file, err := os.Open(backup);
                    if err != nil {
                        displayMessage(window, "打开备份文件出错。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    defer file.Close();
                    buff := make([]byte, 65536);
                    size, err := file.Read(buff);
                    if err != nil || size < len(appId) + 55 {
                        displayMessage(window, "打开备份文件出错。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    if bytes.Compare(buff[0:len(appId) + 1], []byte(appId + "\n")) != 0 {
                        displayMessage(window, "选择文件不是备份文件。", gtk.MESSAGE_WARNING);
                        return;
                    }
                    flag := buff[len(appId) + 1];
                    compressed = (flag & 2) == 2;
                    encrypted = (flag & 1) == 1;
                    totalFiles = int(binary.BigEndian.Uint32(buff[len(appId) + 2:len(appId) + 6]));
                    metaLen := int(binary.BigEndian.Uint64(buff[len(appId) + 6:len(appId) + 14]));
                    metaData = buff[len(appId) + 14:len(appId) + 14 + metaLen];
                    if bytes.Compare(metaData[:8], []byte("source: ")) == 0 {
                        originaDir = string(bytes.Split(metaData[8:], []byte("\n"))[0]);
                    }
                    file.Close();
                    fileOffset = len(appId) + 14 + metaLen;
                }
                key := [16]byte{};
                if encrypted {
                    response := 0;
                    dlgPassword, err := createPasswordDialog(window, metaData, &response, &key, &compressed,
                        &originaDir);
                    if err != nil {
                        displayMessage(window, "创建密码输入对话框失败。", gtk.MESSAGE_ERROR);
                        return;
                    }
                    dlgPassword.Run();
                    dlgPassword.Destroy();
                    switch response {
                        case 0:
                            break;
                        case 1:
                            displayMessage(window, "密码错误", gtk.MESSAGE_WARNING);
                            return;
                        case 2:
                            return;
                        case 3:
                            displayMessage(window, "获取密码出错", gtk.MESSAGE_ERROR);
                            return;
                        case 4:
                            displayMessage(window, "创建解密器出错", gtk.MESSAGE_ERROR);
                            return;
                    }
                }
                if !newSource {
                    source = originaDir;
                    edtVerifyOriginaDir.SetText(source);
                }
                edtVerifyBackup.SetSensitive(false);
                btnSelectVerifyBackup.SetSensitive(false);
                chkVerifyNewSource.SetSensitive(false);
                lblVerifyDirLabel.SetSensitive(false);
                edtVerifyOriginaDir.SetSensitive(false);
                btnSelectVerifyDir.SetSensitive(false);
                btnVerify.SetSensitive(false);
                muVerify.Lock();
                cancelVerify = false;
                verifing = true;
                muVerify.Unlock();
                verifyWindow, bufVerifyResult, boxVerifyResult, prgVerify, btnVerifyCancel, err :=
                    showVerify(window, backup, source);
                if err != nil {
                    displayMessage(window, "创建验证窗口错误", gtk.MESSAGE_ERROR);
                    return;
                }
                go func () {
                    canceled, err := verify(backup, source, compressed, encrypted, key,
                        bufVerifyResult, boxVerifyResult, prgVerify, totalFiles, fileOffset);
                    muVerify.Lock();
                    verifing = false;
                    muVerify.Unlock();
                    glib.IdleAdd(func() {
                        if canceled {
                            displayMessage(verifyWindow, "取消验证操作。", gtk.MESSAGE_WARNING);
                            endIter := bufVerifyResult.GetEndIter();
                            bufVerifyResult.InsertMarkup(endIter, "\n<span color=\"orange\"> 验证操作已取消</span>\n");
                        } else if err != nil {
                            displayMessage(verifyWindow, "验证出错：" + err.Error(), gtk.MESSAGE_ERROR);
                            endIter := bufVerifyResult.GetEndIter();
                            bufVerifyResult.InsertMarkup(endIter, "\n<span color=\"red\"> 验证操作出错</span>\n");
                        } else {
                            prgVerify.SetFraction(1);
                            displayMessage(verifyWindow, "验证完成。", gtk.MESSAGE_INFO);
                            endIter := bufVerifyResult.GetEndIter();
                            bufVerifyResult.InsertMarkup(endIter, "\n<span color=\"green\"> 验证操作已完成</span>\n");
                        }
                        boxVerifyResult.ScrollToIter(bufVerifyResult.GetEndIter(), 0, true, 0, 0);
                        btnVerifyCancel.SetLabel("关闭");
                        edtVerifyBackup.SetSensitive(true);
                        btnSelectVerifyBackup.SetSensitive(true);
                        chkVerifyNewSource.SetSensitive(true);
                        useNewSource := chkVerifyNewSource.GetActive();
                        lblVerifyDirLabel.SetSensitive(useNewSource);
                        edtVerifyOriginaDir.SetSensitive(useNewSource);
                        btnSelectVerifyDir.SetSensitive(useNewSource);
                        btnVerify.SetSensitive(true);
                        /*
                        edtVerifyBackup.SetText("");
                        chkVerifyNewSource.SetActive(false);
                        edtVerifyOriginaDir.SetText("");
                        lblVerifyDirLabel.SetSensitive(false);
                        edtVerifyOriginaDir.SetSensitive(false);
                        btnSelectVerifyDir.SetSensitive(false);
                        */
                    });
                }();
            });

            // 显示主窗口
            app.AddWindow(window);
            window.ShowAll();
            prgBackup.SetVisible(false);
            prgRestore.SetVisible(false);
        }
    })
    // 启动程序
    os.Exit(app.Run(os.Args));
}

// =====================================================================================================================

/**
 * 打开目录选择对话框，选择一个目录 (或文件)。
 *
 * - param title     string, 对话框标题
 * - param parent    *gtk.Window, 对话框所属父窗口
 * - param editBox   *gtk.Entry, 选择目录后，目录路径要填写的输入框
 * - param allowFile bool, 是否允许选择文件
 */
func selectFolder(title string, parent *gtk.Window, editBox *gtk.Entry, allowFile bool) {
    const RESPONSE_SELECT = 0;
    action := gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER;
    selectButton := "选择";
    if allowFile {
        action = gtk.FILE_CHOOSER_ACTION_OPEN;
        selectButton = "打开";
    }
    dlgOpen, _ := gtk.FileChooserDialogNewWith2Buttons(title, parent, action,
        selectButton, gtk.RESPONSE_OK, "取消", gtk.RESPONSE_CANCEL);
    if allowFile {
        dlgOpen.AddButton("选择", RESPONSE_SELECT);
    }
    home, err := os.UserHomeDir();
    if err == nil {
        dlgOpen.SetCurrentFolder(home);
    }
    init, err := editBox.GetText();
    init = strings.Trim(init, "\n\r\t ");
    if err == nil {
        if isFolder(init) {
            dlgOpen.SetCurrentFolder(init);
        } else if isFile(init) {
            dlgOpen.SetCurrentFolder(filepath.Dir(init));
        }
    }
    switch dlgOpen.Run() {
        case gtk.RESPONSE_OK, RESPONSE_SELECT:
            editBox.SetText(dlgOpen.GetFilename());
        default:
            break;
    }
    dlgOpen.Destroy();
}


/**
 * 显示消息对话框。
 *
 * -param parent      *gtk.Window, 对话框所属父窗口
 * -param message     string, 对话框消息内容
 * -param messageType MessageType, 对话框消息类型：gtk.MESSAGE_INFO/gtk.MESSAGE_WARNING/gtk.MESSAGE_ERROR
 */
func displayMessage(parent *gtk.Window, message string, messageType gtk.MessageType) {
    dialog := gtk.MessageDialogNew(parent, gtk.DIALOG_MODAL | gtk.DIALOG_DESTROY_WITH_PARENT,
        messageType, gtk.BUTTONS_OK, message);
    dialog.Run();
    dialog.Destroy();
}


/**
 * 创建密码输入对话框。
 *
 * - param parent     *gtk.Window, 所属父窗口
 * - param data       []byte, 用来验证密码和获取原始文件夹的加密数据
 * - param response   *int, 返回操作结果 (0: 密码正确, 1: 密码错误, 2: 取消操作, 3, 4: 发生错误)
 * - param key        *[16]byte, 输入密码正确时, 返回正确的密钥
 * - param originaDir *string，输入密码正确时, 返回原始文件夹
 */
func createPasswordDialog(parent *gtk.Window, data []byte, response *int, key *[16]byte, compressed *bool,
    originaDir *string) (*gtk.Dialog, error) {
    dialog, err := gtk.DialogNew();
    if err != nil {
        return nil, err;
    }
    dialog.SetResizable(false);
    dialog.SetDefaultSize(350, 0);
    dialog.SetTitle("输入解密密码");
    okButton, err := dialog.AddButton("确定", gtk.RESPONSE_OK);
    if err != nil {
        return nil, err;
    }
    okButton.SetMarginBottom(10);
    okButton.SetMarginEnd(5);
    cancelButton, err := dialog.AddButton("取消", gtk.RESPONSE_CANCEL);
    if err != nil {
        return nil, err;
    }
    cancelButton.SetMarginBottom(10);
    cancelButton.SetMarginEnd(10);
    grid, err := gtk.GridNew();
    if err != nil {
        return nil, err;
    }
    grid.SetHExpand(true);
    grid.SetVExpand(false);
    grid.SetBorderWidth(10);
    grid.SetColumnSpacing(10);
    grid.SetRowSpacing(10);
    area, err := dialog.GetContentArea();
    if err != nil {
        return nil, err;
    }
    area.Add(grid);
    area.SetHExpand(true)
    area.SetVExpand(true)
    lbl, err := gtk.LabelNew("密码：");
    if err != nil {
        return nil, err;
    }
    grid.Attach(lbl, 0, 1, 1, 1);
    password, err := gtk.EntryNew();
    if err != nil {
        return nil, err;
    }
    password.SetVisibility(false);
    password.SetHExpand(true);
    password.SetVExpand(true);
    password.Connect("activate", func() {
        dialog.Emit("response", gtk.RESPONSE_OK, nil);
    })
    grid.Attach(password, 1, 1, 1, 1);
    dialog.SetTransientFor(parent);
    dialog.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT);
    dialog.ShowAll();
    dialog.Connect("response", func(_ glib.IObject, rt gtk.ResponseType) {
        switch rt {
            case gtk.RESPONSE_OK:
                pwdText, err := password.GetText();
                if err != nil {
                    *response = 3;
                    return;
                }
                *key = md5.Sum([]byte(appId + ":" + pwdText));
                cipher, err := rc4.NewCipher((*key)[:]);
                if err != nil {
                    *response = 4;
                    return;
                }
                buff := make([]byte, len(data));
                cipher.XORKeyStream(buff, data);
                meta := string(buff);
                verified := strings.HasPrefix(meta, "source: ");
                *originaDir = strings.SplitN(meta[8:], "\n", 2)[0];
                if strings.HasSuffix(meta, "\ncompressed: false\n") {
                    *compressed = false;
                } else if strings.HasSuffix(meta, "\ncompressed: true\n") {
                    *compressed = true;
                }
                if verified {
                    *response = 0;
                } else {
                    *response = 1;
                }
            case gtk.RESPONSE_CANCEL, gtk.RESPONSE_DELETE_EVENT:
                *response = 2;
        }
    })
    return dialog, nil;
}


/**
 * 判断路径是否是存在的文件。
 *
 * - param path string, 文件路径
 *
 * - return bool, 路径是否是存在的文件
 */
func isFile(path string) bool {
    fi, err := os.Stat(path);
    if err != nil { return false; }
    return !fi.IsDir();
}


/**
 * 判断路径是否是存在的文件夹。
 *
 * - param path string, 文件夹路径
 *
 * - return bool, 路径是否是存在的文件夹
 */
func isFolder(path string) bool {
    fi, err := os.Stat(path);
    if err != nil { return false; }
    return fi.IsDir();
}


/**
 * 判断文件夹是否是空文件夹。
 *
 * - param path string, 文件夹路径
 *
 * - return bool, 路径是否存在且为空文件夹
 */
func isEmptyFolder(path string) bool {
    fi, err := ioutil.ReadDir(path);
    if err != nil { return false; }
    return len(fi) == 0;
}


/**
 * 定义错误类型结构。
 */
type Error struct {
    ErrMesg string;      // 错误信息
}


/**
 * 构造错误对象。
 *
 * - param code int, 错误代码
 * - param mesg string, 错误消息
 *
 * - return *Error, 返回构造的错误对象
 */
func NewError(mesg string) *Error {
    return &Error{ ErrMesg: mesg }
}


/**
 * 获取错误对象的错误消息。
 *
 * - return string, 错误消息
 */
func (err *Error) Error() string {
    return err.ErrMesg;
}

// =====================================================================================================================

/**
 * 执行备份操作。
 *
 * - param source      string, 备份源目录
 * - param target      string, 备份目标路径 (文件或目录)
 * - param compressed  bool, 是否压缩
 * - param packed      bool, 是否打包
 * - param encrypted   bool, 是否加密
 * - param password    string, 加密时，指定密码
 * - param parent      *gtk.Window, 所属父窗口
 * - param progressBar *gtk.ProgressBar, 进度条
 *
 * - return canceled   bool, 备份操作是否被取消
 * - return err        error, 备份过程中发生的错误
 */
func backup(source string, target string, compressed bool, packed bool, encrypted bool, password string,
            parent *gtk.Window, progressBar *gtk.ProgressBar) (canceled bool, err error) {
    var key [16]byte;
    if encrypted {
        key = md5.Sum([]byte(appId + ":" + password));
    }
    if packed {
        // 创建备份包文件
        pkg, err := os.OpenFile(target, os.O_WRONLY | os.O_CREATE, 0644);
        if err != nil {
            return false, NewError("创建备份文件出错: " + err.Error()); ;
        }
        defer pkg.Close();
        // 写标志 (appId + "\n")
        _, err = pkg.WriteString(appId + "\n");
        if err != nil {
            return false, NewError("写备份元数据出错: " + err.Error());
        }
        // 写头 (5 字节), 包括标志 (D5: 元数据, D1: 压缩, D0: 加密)，文件总数 (Uint32, 高字节在前)
        head := [5]byte{32, 0, 0, 0, 0};
        if encrypted { head[0] = head[0] | 1; }
        if compressed { head[0] = head[0] | 2; }
        _, err = pkg.Write(head[:]);
        if err != nil {
            return false, NewError("写备份元数据出错: " + err.Error());
        }
        // 写元数据内容长度 (uint64，8 字节)
        meta := []byte("source: " + source + "\n" + "time: " + time.Now().Format(time.RFC3339) + "\n");
        metaLen := len(meta);
        var metaLenArr [8]byte;
        binary.BigEndian.PutUint64(metaLenArr[0:8], uint64(metaLen));
        _, err = pkg.Write(metaLenArr[:])
        if err != nil {
            return false, NewError("写备份元数据出错: " + err.Error());
        }
        // 写元数据内容
        if encrypted {
            cipher, err := rc4.NewCipher(key[:]);
            if err == nil {
                buff := make([]byte, metaLen);
                cipher.XORKeyStream(buff, meta);
                _, err = pkg.Write(buff);
            }
        } else {
            _, err = pkg.Write(meta);
        }
        if err != nil {
            return false, NewError("写备份元数据出错: " + err.Error());
        }
        // 打包文件内容
        var backedup int = 0;
        var total int = 1;
        canceled, err := packDir(source, pkg, encrypted, key, compressed, progressBar, &backedup, &total);
        if canceled || err != nil {
            return canceled, err;
        }
        // 写文件总数
        _, err = pkg.Seek(int64(len(appId) + 2), 0);
        if err != nil {
            return false, err;
        }
        totalBuf := [4]byte{};
        binary.BigEndian.PutUint32(totalBuf[0:4], uint32(total));
        _, err = pkg.Write(totalBuf[:]);
        if err != nil {
            return false, err;
        }
        pkg.Seek(0, 2);
        return false, nil;
    } else {
        if !isFolder(target) {
            err := os.MkdirAll(target, os.ModePerm);
            if err != nil {
                return false, NewError("创建备份目录出错: " + err.Error());
            };
        }
        err := writeMetaFile(target, source, encrypted, key, compressed);
        if err != nil {
            return false, NewError("写备份元数据出错: " + err.Error());
        }
        var backedup int = 0;
        var total int = 1;
        return backupDir(source, target, encrypted, key, compressed, progressBar, &backedup, &total);
    }
}

// ---------------------------------------------------------------------------------------------------------------------

/**
 * 创建备份元数据文件。
 *
 * - param target      string, 备份目标目录
 * - param source      string, 备份源目录
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 *
 * - return            error, 返回操作错误
 */
func writeMetaFile(target string, source string, encrypted bool, key [16]byte, compressed bool) error {
    meta, err := os.Create(path.Join(target, "._backup.meta"));
    if err != nil {
        return err;
    }
    defer meta.Close();
    _, err = meta.WriteString(appId + "\n");
    if err != nil {
        return err;
    }
    compressedText := "false";
    if compressed {
        compressedText = "true"
    }
    text := "source: " + source + "\n" + "time: " + time.Now().Format(time.RFC3339) + "\n" + "compressed: " +
        compressedText + "\n";
    if encrypted {
        cipher, err := rc4.NewCipher(key[:]);
        if err != nil {
            return err;
        }
        buff0 := []byte(text);
        buff1 := make([]byte, len(buff0));
        cipher.XORKeyStream(buff1, buff0);
        _, err = meta.Write(buff1);
    } else {
        _, err = meta.WriteString(text);
    }
    return err;
}


/**
 * 备份一个目录及其下子目录。
 *
 * - param source      string, 源目录
 * - param target      string, 目标目录
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 * - param progressBar *gtk.ProgressBar, 进度条
 * - param backedup    *int, 已备份文件和目录数
 * - param total       *int, 总文件和目录数
 *
 * - return canceled   bool, 备份操作是否被取消
 * - return err        error, 备份过程中发生的错误
 */
func backupDir(source string, target string, encrypted bool, key [16]byte, compressed bool,
               progressBar *gtk.ProgressBar, backedup *int, total *int) (caneceled bool, err error) {
    // 打开目录
    dir, err := ioutil.ReadDir(source);
    if err != nil { return false, NewError("打开目录出错: " + err.Error()); }
    *total += len(dir);
    glib.IdleAdd(func() { progressBar.SetFraction(float64(*backedup) / float64(*total)); });
    // 依次备份目录下内容
    for _, fi := range dir {
        // 检测取消
        muBackup.RLock();
        canceled := cancelBackup;
        muBackup.RUnlock();
        if canceled {
            return true, nil;
        }
        // 执行备份
        if fi.IsDir() {
            // 创建目录
            err = os.Mkdir(path.Join(target, fi.Name()), os.ModePerm);
            if err != nil {
                return false, NewError("创建目录出错: " + err.Error());
            }
            // 备份目录下子项
            cancel, err := backupDir(path.Join(source, fi.Name()), path.Join(target, fi.Name()), encrypted, key,
                                     compressed, progressBar, backedup, total);
            if cancel {
                return true, nil;
            } else if err != nil {
                return false, err;
            }
        } else {
            // 备份文件
            err := backupFile(path.Join(source, fi.Name()), path.Join(target, fi.Name()), encrypted, key, compressed);
            if err != nil {
                return false, NewError("备份文件出错: " + err.Error());
            }
        }
        *backedup++;
        glib.IdleAdd(func() { progressBar.SetFraction(float64(*backedup) / float64(*total)); });
    }
    return false, nil;
}


/**
 * 备份一个文件。
 *
 * - param source      string, 源路径
 * - param target      string, 目标路径
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 *
 * - return            error, 返回操作错误
 */
func backupFile(source string, target string, encrypted bool, key [16]byte, compressed bool) error {
    // 打开待备份文件
    src, err := os.Open(source)
    if err != nil {
        return err;
    }
    defer src.Close()
    // 创建备份文件
    dst, err := os.OpenFile(target, os.O_WRONLY | os.O_CREATE, 0644);
    if err != nil {
        return err;
    }
    defer dst.Close();
    // 开始写备份文件
    if !encrypted && !compressed {
        _, err := io.Copy(dst, src);
        return err;
    } else {
        // 创建加密器
        var cipher *rc4.Cipher;
        if encrypted {
            cipher, err = rc4.NewCipher(key[:]);
            if err != nil {
                return err;
            }
        }
        // 处理内容
        var buf []byte;
        buff0 := make([]byte, 65536);
        buff1 := make([]byte, 65536);
        for {
            size, err := src.Read(buff0);
            if err == io.EOF || size < 0 {
                break;
            } else if err != nil {
                return err;
            }
            if compressed {
                buf, err = compressGZIP(buff0[0:size]);
                if encrypted {
                    buff1 = make([]byte, len(buf));
                    cipher.XORKeyStream(buff1, buf);
                    _, err = dst.Write(buff1);
                } else {
                    _, err = dst.Write(buf);
                }
            } else {
                if encrypted {
                    cipher.XORKeyStream(buff1, buff0);
                    _, err = dst.Write(buff1[0:size]);
                } else {
                    _, err = dst.Write(buff0[0:size]);
                }
            }
            if err != nil {
                return err;
            }
        }
        return nil;
    }
}

// ---------------------------------------------------------------------------------------------------------------------

/**
 * 打包一个目录及其下子目录。
 *
 * - param source      string, 源目录
 * - param file        *File, 打包文件
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 * - param progressBar *gtk.ProgressBar, 进度条
 * - param backedup    *int, 已备份文件和目录数
 * - param total       *int, 总文件和目录数
 *
 * - return canceled   bool, 打包操作是否被取消
 * - return err        error, 打包过程中发生的错误
 */
func packDir(source string, file *os.File, encrypted bool, key [16]byte, compressed bool,
             progressBar *gtk.ProgressBar, backedup *int, total *int) (canceled bool, err error) {
    // 打开目录
    dir, err := ioutil.ReadDir(source);
    if err != nil { return false, NewError("打开目录出错: " + err.Error()); }
    *total += len(dir);
    glib.IdleAdd(func() { progressBar.SetFraction(float64(*backedup) / float64(*total)); });
    // 依次打包目录下内容
    for _, fi := range dir {
        // 检查取消操作
        muBackup.RLock();
        canceled := cancelBackup;
        muBackup.RUnlock();
        if canceled {
            return true, nil;
        }
        // 打包一个子项
        if fi.IsDir() {
            // 写目录头, 包括标志 (D4: 退出目录, D3D2: 01-目录, D1: 压缩, D0: 加密)，名称长度 (uint16, 高字节在前)
            head := [3]byte{4, 0, 0};
            nameBuf := []byte(fi.Name());
            head[1] = byte((uint16(len(nameBuf)) & 0xff00) >> 8);
            head[2] = byte(uint16(len(nameBuf)) & 0x00ff);
            _, err := file.Write(head[:]);
            if err != nil {
                return false, NewError("写目录数据出错: " + err.Error());
            }
            // 写目录名称
            _, err = file.Write(nameBuf);
            if err != nil {
                return false, NewError("写目录数据出错: " + err.Error());
            }
            // 打包子目录
            cancel, err := packDir(path.Join(source, fi.Name()), file, encrypted, key, compressed,
                                   progressBar, backedup, total);
            if cancel {
                return true, nil;
            } else if err != nil {
                return false, err;
            }
            // 写退出目录, 包括标志 (D4: 退出目录, D3D2: 01-目录, D1: 压缩, D0: 加密)，名称长度 (uint16, 0）
            head = [3]byte{20, 0, 0};
            _, err = file.Write(head[:]);
            if err != nil {
                return false, NewError("写目录数据出错: " + err.Error());
            }
        } else {
            // 打包一个文件
            err = packFile(path.Join(source, fi.Name()), fi.Name(), file, encrypted, key, compressed);
            if err != nil {
                return false, NewError("写文件出错: " + err.Error());
            }
        }
        *backedup++;
        glib.IdleAdd(func() { progressBar.SetFraction(float64(*backedup) / float64(*total)); });
    }
    return false, nil;
}


/**
 * 打包一个文件。
 *
 * - param source      string, 源路径 (含文件名)
 * - param name        string, 源文件名
 * - param file        *File, 打包文件
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 *
 * - return err        error, 打包过程中发生的错误
 */
func packFile(source string, name string, file *os.File, encrypted bool, key [16]byte, compressed bool) error {
    // 打开带备份文件
    src, err := os.Open(source)
    if err != nil {
        return err;
    }
    defer src.Close();
    // 写头, 包括标志 (D4: 退出目录, D3D2: 00-文件/01-目录/02-连接/03-管道, D1: 压缩, D0: 加密)，名称长度 (uint16, 高字节在前）
    head := [3]byte{0, 0, 0};
    if encrypted { head[0] = 1; }
    if compressed { head[0] = head[0] | 2; }
    nameBuf := []byte(name);
    head[1] = byte((uint16(len(nameBuf)) & 0xff00) >> 8);
    head[2] = byte(uint16(len(nameBuf)) & 0x00ff);
    _, err = file.Write(head[:]);
    if err != nil {
        return err;
    }
    // 写名称
    _, err = file.Write(nameBuf);
    if err != nil {
        return err;
    }
    // 写内容长度
    position, err := file.Seek(0, 1); // 保存当前位置
    contentLen := [8]byte{};
    _, err = file.Write(contentLen[:]);
    if err != nil {
        return err;
    }
    // 创建加密器
    var cipher *rc4.Cipher;
    if encrypted {
        cipher, err = rc4.NewCipher(key[:]);
        if err != nil {
            return err;
        }
    }
    // 处理内容
    var buf []byte;
    buff0 := make([]byte, 65536);
    buff1 := make([]byte, 65536);
    var totalSize uint64 = 0;
    for {
        size, err := src.Read(buff0);
        if err == io.EOF || size < 0 {
            break;
        } else if err != nil {
            return err;
        }
        if compressed {
            buf, err = compressGZIP(buff0[0:size]);
            if encrypted {
                buff1 = make([]byte, len(buf));
                cipher.XORKeyStream(buff1, buf);
                _, err = file.Write(buff1);
            } else {
                _, err = file.Write(buf);
            }
            totalSize += uint64(len(buf));
        } else {
            if encrypted {
                cipher.XORKeyStream(buff1, buff0);
                _, err = file.Write(buff1[0:size]);
            } else {
                _, err = file.Write(buff0[0:size]);
            }
            totalSize += uint64(size);
        }
        if err != nil {
            return err;
        }
    }
    // 写内容长度
    binary.BigEndian.PutUint64(contentLen[0:8], totalSize);
    file.Seek(position, 0);
    _, err = file.Write(contentLen[:]);
    if err != nil {
        return err;
    }
    file.Seek(0, 2);
    return nil;
}

// =====================================================================================================================

/**
 * 执行恢复操作。
 *
 * - param backup      string, 备份保存路径
 * - param target      string, 恢复目标目录
 * - param compressed  bool, 是否压缩
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param progressBar *gtk.ProgressBar, 进度条
 * - param totalFiles  int, 总文件和目录数 (用来显示进度条)
 * - param fileOffset  int, 实际备份文件偏移
 *
 * - return canceled   bool, 恢复操作是否被取消
 * - return err        error, 恢复过程中发生的错误
 */
func restore(backup string, target string, compressed bool, encrypted bool, key [16]byte, progressBar *gtk.ProgressBar,
             totalFiles int, fileOffset int) (canceled bool, err error) {
    // 创建恢复文件夹
    if !isFolder(target) {
        err := os.MkdirAll(target, os.ModePerm);
        if err != nil {
            return false, NewError("创建恢复目录出错: " + err.Error());
        };
    }
    // 恢复
    if isFile(backup) {
        // 打开备份包
        file, err := os.Open(backup);
        if err != nil {
            return false, err;
        }
        // 跳转至备份文件部分
        _, err = file.Seek(int64(fileOffset), 0);
        if err != nil {
            return false, err;
        }
        // 恢复所有文件
        return unpack(file, target, compressed, encrypted, key, progressBar, totalFiles);
    } else {
        // 恢复备份目录
        var restored int = 0;
        var total int = 1;
        return restoreDir(backup, target, encrypted, key, compressed, progressBar, &restored, &total);
    }
}

// ---------------------------------------------------------------------------------------------------------------------

/**
 * 恢复一个目录及其下子目录。
 *
 * - param source      string, 源目录
 * - param target      string, 目标目录
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 * - param progressBar *gtk.ProgressBar, 进度条
 * - param restored    *int, 已恢复文件和目录数
 * - param total       *int, 总文件和目录数 (用来显示进度条)
 *
 * - return canceled   bool, 恢复操作是否被取消
 * - return err        error, 恢复过程中发生的错误
 */
func restoreDir(source string, target string, encrypted bool, key [16]byte, compressed bool,
                progressBar *gtk.ProgressBar, restored *int, total *int) (canceled bool, err error) {
    // 打开目录
    dir, err := ioutil.ReadDir(source);
    if err != nil {
        return false, NewError("打开目录出错: " + err.Error());
    }
    *total += len(dir);
    glib.IdleAdd(func() { progressBar.SetFraction(float64(*restored) / float64(*total)); });
    // 依次恢复目录下子项
    for _, fi := range dir {
        // 检测取消
        muRestore.RLock();
        canceled := cancelRestore;
        muRestore.RUnlock();
        if canceled {
            return true, nil;
        }
        //开始恢复
        if fi.IsDir() {
            // 删除与目录同名文件
            dir := path.Join(target, fi.Name());
            if isFile(dir) {
                err = os.Remove(dir);
                if err != nil {
                    return false, NewError("删除文件出错: " + err.Error());
                }
            }
            // 不存在则创建
            if !isFolder(dir) {
                err = os.Mkdir(dir, os.ModePerm);
                if err != nil {
                    return false, NewError("创建目录出错: " + err.Error());
                }
            }
            // 恢复目录下子项
            cancel, err := restoreDir(path.Join(source, fi.Name()), path.Join(target, fi.Name()), encrypted, key,
                                     compressed, progressBar, restored, total);
            if cancel {
                return true, nil;
            } else if err != nil {
                return false, err;
            }
        } else {
            if fi.Name() == "._backup.meta" {
                continue;
            }
            // 恢复一个文件
            err = restoreFile(path.Join(source, fi.Name()), path.Join(target, fi.Name()), encrypted, key, compressed);
            if err != nil {
                return false, NewError("备份文件出错: " + err.Error());
            }
        }
        *restored++;
        glib.IdleAdd(func() { progressBar.SetFraction(float64(*restored) / float64(*total)); });
    }
    return false, nil;
}


/**
 * 恢复一个文件。
 *
 * - param source      string, 源路径
 * - param target      string, 目标路径
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 *
 * - return err        error, 恢复过程中发生的错误
 */
func restoreFile(source string, target string, encrypted bool, key [16]byte, compressed bool) error {
    // 打开备份文件
    src, err := os.Open(source)
    if err != nil {
        return err;
    }
    defer src.Close();
    // 如果存在则删除同名文件或目录
    if isFile(target) || isFolder(target) {
        err = os.Remove(target);
        if err != nil {
            return err;
        }
    }
    // 创建恢复文件
    dst, err := os.OpenFile(target, os.O_WRONLY | os.O_CREATE, 0644);
    if err != nil {
        return err;
    }
    defer dst.Close();
    //开始恢复
    if !encrypted && !compressed {
        _, err := io.Copy(dst, src);
        return err;
    } else {
        // 创建加密器
        var cipher *rc4.Cipher;
        if encrypted {
            cipher, err = rc4.NewCipher(key[:]);
            if err != nil {
                return err;
            }
        }
        // 处理内容
        buff0 := make([]byte, 65536);
        buff1 := make([]byte, 65536);
        for {
            size, err := src.Read(buff0);
            if err == io.EOF || size < 0 {
                break;
            } else if err != nil {
                return err;
            }
            if compressed {
                var buf []byte;
                if encrypted {
                    cipher.XORKeyStream(buff1, buff0);
                    buf, err = decompressGZIP(buff1[0:size]);
                } else {
                    buf, err = decompressGZIP(buff0[0:size]);
                }
                _, err = dst.Write(buf);
            } else {
                if encrypted {
                    cipher.XORKeyStream(buff1, buff0);
                    _, err = dst.Write(buff1[0:size]);
                } else {
                    _, err = dst.Write(buff0[0:size]);
                }
            }
            if err != nil {
                return err;
            }
        }
        return nil;
    }
}

// ---------------------------------------------------------------------------------------------------------------------

/**
 * 解包。
 *
 * - param file        *os.File, 打包备份文件
 * - param target      string, 恢复目标目录
 * - param compressed  bool, 是否压缩
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param progressBar *gtk.ProgressBar, 进度条
 * - param totalFiles  int, 总文件和目录数 (用来显示进度条)
 *
 * - return canceled   bool, 恢复操作是否被取消
 * - return err        error, 恢复过程中发生的错误
 */
func unpack(file *os.File, target string, compressed bool, encrypted bool, key [16]byte, progressBar *gtk.ProgressBar,
            totalFiles int) (canceled bool, err error) {
    var currentDir string = target;
    for {
        // 检测取消
        muRestore.RLock();
        canceled := cancelRestore;
        muRestore.RUnlock();
        if canceled {
            return true, nil;
        }
        // 读头
        head := make([]byte, 3);
        size, err := file.Read(head);
        if err == io.EOF || size < 0 {
            break;
        } else if size < 3 {
            return false, NewError("读文件头错误");
        } else if err != nil {
            return false, err;
        }
        // 处理头
        restored := 0;
        if (head[0] >> 2) == 5 {
            // 退出目录
            currentDir, _ = filepath.Split(currentDir);
            if strings.HasSuffix(currentDir, string(os.PathSeparator)) {
                currentDir = currentDir[0:len(currentDir) - 1];
            }
            restored++;
            glib.IdleAdd(func() { progressBar.SetFraction(float64(restored) / float64(totalFiles)); });
        } else {
            // 取得名称
            nameLen := int(uint((head[1] << 8) | head[2]));
            nameBuf := make([]byte, nameLen);
            size, err = file.Read(nameBuf);
            if size < nameLen {
                return false, NewError("读文件名称错误");
            }
            if err != nil {
                return false, err;
            }
            name := string(nameBuf);
            //处理路径
            switch head[0] >> 2 {
                case 0:
                    // 恢复文件
                    err = unpackFile(currentDir, name, file, compressed, encrypted, key);
                    if err != nil {
                        return false, err;
                    }
                    restored++;
                    glib.IdleAdd(func() { progressBar.SetFraction(float64(restored) / float64(totalFiles)); });
                case 1:
                    // 恢复目录，删除同名文件，不存在则创建
                    currentDir = path.Join(currentDir, name);
                    if isFile(currentDir) {
                        err = os.Remove(currentDir);
                        if err != nil {
                            return false, err;
                        }
                    }
                    if !isFolder(currentDir) {
                        err = os.Mkdir(currentDir, os.ModePerm);
                        if err != nil {
                            return false, err;
                        }
                    }
                case 2: // 连接
                    break;
                case 3: // 管道
                    break;
            }
        }
    }
    return false, nil;
}


/**
 * 解包一个文件。
 *
 * - param currentDir  string, 正在恢复的目录路径
 * - param filename    string, 正在恢复的文件名
 * - param source      *os.File, 恢复的打包备份文件
 * - param compressed  bool, 是否压缩
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 *
 * - return err        error, 恢复过程中发生的错误
 */
func unpackFile(currentDir string, filename string, source *os.File, compressed bool, encrypted bool,
                key [16]byte) error {
    // 创建文件
    filepath := path.Join(currentDir, filename);
    if isFile(filepath) || isFolder(filepath) {
        err := os.Remove(filepath);
        if err != nil {
            return err;
        }
    }
    dst, err := os.OpenFile(filepath, os.O_WRONLY | os.O_CREATE, 0644);
    if err != nil {
        return err;
    }
    defer dst.Close();
    // 创建加密器
    var cipher *rc4.Cipher;
    if encrypted {
        cipher, err = rc4.NewCipher(key[:]);
        if err != nil {
            return err;
        }
    }
    // 文件长度
    lenBuf := make([]byte, 8);
    size, err := source.Read(lenBuf);
    if size < 8 {
        return NewError("获取文件长度错误");
    } else if err != nil {
        return err;
    }
    contLen := uint64(binary.BigEndian.Uint64(lenBuf[0:8]));
    // 读文件内容
    buff0 := make([]byte, 65536);
    buff1 := make([]byte, 65536);
    for {
        var readLen uint64 = 65536;
        if contLen < 65536 {
            readLen = contLen;
            buff0 = make([]byte, readLen);
            buff1 = make([]byte, readLen);
        }
        size, err := source.Read(buff0);
        if uint64(size) < readLen {
            return NewError("获取文件内容错误");
        }
        if err != nil {
            return err;
        }
        contLen -= readLen;
        if compressed {
            var buf []byte;
            if encrypted {
                cipher.XORKeyStream(buff1, buff0);
                buf, err = decompressGZIP(buff1[0:size]);
            } else {
                buf, err = decompressGZIP(buff0[0:size]);
            }
            _, err = dst.Write(buf);
        } else {
            if encrypted {
                cipher.XORKeyStream(buff1, buff0);
                _, err = dst.Write(buff1[0:size]);
            } else {
                _, err = dst.Write(buff0[0:size]);
            }
        }
        if err != nil {
            return err;
        } else if contLen <= 0 {
            return nil;
        }
    }
}

// =====================================================================================================================

/**
 * 显示验证结果窗口。
 *
 * - param parent        *gtk.Window, 所属父窗口
 * - param backup        string, 备份保存路径
 * - param source        string, 参考原始备份路径
 *
 * - return window       *gtk.Window, 返回验证结果显示窗口
 * - return resultBuf    *gtk.TextBuffer, 返回验证结果输出缓存
 * - return resultBox    *gtk.TextView, 返回验证结果输出框
 * - return progressBar  *gtk.ProgressBar, 返回进度条
 * - return cancelButton *gtk.Button, 进度条取消/关闭按钮
 * - return err          error, 返回显示窗口时发生的错误
 */
func showVerify(parent *gtk.Window, backup string, source string) (window *gtk.Window, resultBuf *gtk.TextBuffer,
                resultBox *gtk.TextView, progressBar *gtk.ProgressBar, cancelButton *gtk.Button, err error) {
    if builder, err := gtk.BuilderNewFromFile("verify.ui"); err != nil {
        return nil, nil, nil, nil, nil, err;
    } else {
        // 主窗口
        winObj, _ := builder.GetObject("verifyWindow");
        verifyWindow := winObj.(*gtk.Window);

        // 备份路径显示框
        edtBackupDirObj, _ := builder.GetObject("backupPath");
        edtBackupDir := edtBackupDirObj.(*gtk.Entry);
        edtBackupDir.SetText(backup);

        // 备份源路径显示框
        edtSourceDirObj, _ := builder.GetObject("originaPath");
        edtSourceDir := edtSourceDirObj.(*gtk.Entry);
        edtSourceDir.SetText(source);

        // 验证结果显示框
        bufVerifyResultObj, _ := builder.GetObject("resultBuffer");
        bufVerifyResult := bufVerifyResultObj.(*gtk.TextBuffer);
        boxVerifyResultObj, _ := builder.GetObject("verifyResult");
        boxVerifyResult := boxVerifyResultObj.(*gtk.TextView);

        // 验证进度条
        prgVerifyObj, _ := builder.GetObject("verifyProgress");
        prgVerify := prgVerifyObj.(*gtk.ProgressBar);
        prgVerify.SetFraction(0);

        // 验证取消按钮
        btnCancelVerifyObj, _ := builder.GetObject("closeButton");
        btnCancelVerify := btnCancelVerifyObj.(*gtk.Button);
        btnCancelVerify.SetLabel("取消");
        btnCancelVerify.Connect("clicked", func () {
            muVerify.Lock();
            defer muVerify.Unlock();
            if verifing {
                cancelVerify = true;
            } else {
                verifyWindow.Destroy();
            }
        });
        // 显示验证窗口
        verifyWindow.SetTransientFor(parent);
        verifyWindow.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT);
        verifyWindow.ShowAll();
        return verifyWindow, bufVerifyResult, boxVerifyResult, prgVerify, btnCancelVerify, nil;
    }
}


/**
 * 执行验证操作。
 *
 * - param backup          string, 备份保存路径
 * - param source          string, 参考原始备份路径
 * - param compressed      bool, 是否压缩
 * - param encrypted       bool, 是否加密
 * - param key             [16]byte, 加密时，指定密钥
 * - param bufVerifyResult *gtk.TextBuffer, 验证结果输出缓存
 * - param boxVerifyResult *gtk.TextView, 验证结果输出框
 * - param progressBar     *gtk.ProgressBar, 进度条
 * - param totalFiles      int, 总文件和目录数 (用来显示进度条)
 * - param fileOffset      int, 实际备份文件偏移
 *
 * - return canceled       bool, 验证操作是否被取消
 * - return err            error, 验证过程中发生的错误
 */
func verify(backup string, source string, compressed bool, encrypted bool, key [16]byte,
            bufVerifyResult *gtk.TextBuffer, boxVerifyResult *gtk.TextView, progressBar *gtk.ProgressBar,
            totalFiles int, fileOffset int) (canceled bool, err error) {
    if isFolder(backup) {
        // 恢复备份目录
        var verified int = 0;
        var total int = 1;
        return verifyDir(backup, source, encrypted, key, compressed, bufVerifyResult, boxVerifyResult, progressBar,
            &verified, &total);
    } else if isFile(backup) {
        // 打开备份包
        file, err := os.Open(backup);
        if err != nil {
            return false, err;
        }
        // 跳转至备份文件部分
        _, err = file.Seek(int64(fileOffset), 0);
        if err != nil {
            return false, err;
        }
        // 验证所有文件
        canceled, paths, err := verifyPack(file, source, compressed, encrypted, key, bufVerifyResult, boxVerifyResult,
            progressBar, totalFiles);
        if err != nil {
            return false, err;
        } else if canceled {
            return true, nil;
        } else {
            sort.Strings(paths);
            canceled, err := findNewPaths(source, paths, bufVerifyResult, boxVerifyResult);
            if err != nil {
                return false, err;
            } else if canceled {
                return true, nil;
            }
        }
    } else {
        return false, NewError("无效备份");
    }
    return false, nil;
}


/**
 * 输出验证结果。
 *
 * - param source          string, 参考原始备份路径
 * - param backup          string, 备份保存路径
 * - param kind            int, 验证结果 (0: 文件, 2: 目录, 3: 连接, 4: 管道)
 * - param result          int, 验证结果 (1: 修改, 2: 添加, 3: 删除)
 * - param bufVerifyResult *gtk.TextBuffer, 验证结果输出缓存
 * - param boxVerifyResult *gtk.TextView, 验证结果输出框
 *
 * - return err            error, 输出过程中发生的错误
 */
func outputVerifyResult(source string, backup string, kind int, result int, bufVerifyResult *gtk.TextBuffer,
    boxVerifyResult *gtk.TextView) error {
    glib.IdleAdd(func() {
        endIter := bufVerifyResult.GetEndIter();
        switch result {
            case 1: bufVerifyResult.InsertMarkup(endIter, " <span color=\"blue\">修改</span>  ");
            case 2: bufVerifyResult.InsertMarkup(endIter, " <span color=\"green\">添加</span>  ");
            case 3: bufVerifyResult.InsertMarkup(endIter, " <span color=\"red\">删除</span>  ");
        }
        endIter = bufVerifyResult.GetEndIter();
        switch kind {
            case 0: bufVerifyResult.InsertMarkup(endIter, "<span color=\"teal\">文件</span>  ");
            case 1: bufVerifyResult.InsertMarkup(endIter, "<span color=\"purple\">目录</span>  ");
        }
        endIter = bufVerifyResult.GetEndIter();
        bufVerifyResult.InsertMarkup(endIter, "<span color=\"black\">" + source + "</span>\n");
        boxVerifyResult.ScrollToIter(bufVerifyResult.GetEndIter(), 0, true, 0, 0);
    });
    if isFolder(backup) {
        dir, err := ioutil.ReadDir(backup);
        if err != nil {
            return NewError("打开目录出错: " + err.Error());
        }
        // 依次输出目录下子项
        for _, fi := range dir {
            kindSub := 0;
            if fi.IsDir() {
                kindSub = 1;
            }
            err = outputVerifyResult(path.Join(source, fi.Name()), path.Join(backup, fi.Name()), kindSub, result,
                bufVerifyResult, boxVerifyResult);
            if err != nil {
                return err;
            }
        }
    }
    return nil;
}

// ---------------------------------------------------------------------------------------------------------------------

/**
 * 验证一个目录及其下子目录。
 *
 * - param backup          string, 备份保存路径
 * - param source          string, 参考原始备份路径
 * - param encrypted       bool, 是否加密
 * - param key             [16]byte, 加密时，指定密钥
 * - param compressed      bool, 是否压缩
 * - param bufVerifyResult *gtk.TextBuffer, 验证结果输出缓存
 * - param boxVerifyResult *gtk.TextView, 验证结果输出框
 * - param progressBar     *gtk.ProgressBar, 进度条
 * - param verified        *int, 已验证文件和目录数
 * - param total           *int, 总文件和目录数 (用来显示进度条)
 *
 * - return canceled       bool, 验证操作是否被取消
 * - return err            error, 验证过程中发生的错误
 */
func verifyDir(backup string, source string, encrypted bool, key [16]byte, compressed bool,
               bufVerifyResult *gtk.TextBuffer, boxVerifyResult *gtk.TextView, progressBar *gtk.ProgressBar,
               verified *int, total *int) (canceled bool, err error) {
    // 打开源目录
    dir, err := ioutil.ReadDir(source);
    if err != nil {
        return false, NewError("打开源目录出错: " + err.Error());
    }
    *total += len(dir);
    glib.IdleAdd(func() { progressBar.SetFraction(float64(*verified) / float64(*total)); });
    // 依次验证源目录下子项
    for _, fi := range dir {
        // 检测取消
        muVerify.RLock();
        canceled := cancelVerify;
        muVerify.RUnlock();
        if canceled {
            return true, nil;
        }
        //开始验证
        sourceSub := path.Join(source, fi.Name());
        backupSub := path.Join(backup, fi.Name());
        if fi.IsDir() {
            if isFile(backupSub) {
                // 文件 sourceSub 已删除 (不递归)
                outputVerifyResult(sourceSub, "", 0, 3, bufVerifyResult, boxVerifyResult);
                // 目录 sourceSub 已添加（包括子项）
                outputVerifyResult(sourceSub, sourceSub, 1, 2, bufVerifyResult, boxVerifyResult);
            } else if isFolder(backupSub) {
                // 验证子目录
                cancel, err := verifyDir(backupSub, sourceSub, encrypted, key, compressed, bufVerifyResult,
                    boxVerifyResult, progressBar, verified, total);
                if cancel {
                    return true, nil;
                } else if err != nil {
                    return false, err;
                }
            } else {
                // 目录 sourcePath 已添加
                outputVerifyResult(sourceSub, sourceSub, 1, 2, bufVerifyResult, boxVerifyResult);
            }
        } else {
            if isFile(backupSub) {
                // 比较文件 sourcePath/backupSub, 不相同则文件 sourcePath 已修改
                equal, err := verifyFile(backupSub, sourceSub, encrypted, key, compressed);
                if err != nil {
                    return false, err;
                }
                if !equal {
                    // 文件 sourcePath 已修改
                    outputVerifyResult(sourceSub, sourceSub, 0, 1, bufVerifyResult, boxVerifyResult);
                }
            } else if isFolder(backupSub) {
                // 目录 sourceSub 已删除（包括子项）
                outputVerifyResult(sourceSub, backupSub, 1, 3, bufVerifyResult, boxVerifyResult);
                // 文件 sourceSub 已添加
                outputVerifyResult(sourceSub, sourceSub, 0, 2, bufVerifyResult, boxVerifyResult);
            } else {
                // 文件 sourceSub 已添加
                outputVerifyResult(sourceSub, sourceSub, 0, 2, bufVerifyResult, boxVerifyResult);
            }
        }
        *verified++;
        glib.IdleAdd(func() { progressBar.SetFraction(float64(*verified) / float64(*total)); });
    }
    // 打开备份目录
    dir, err = ioutil.ReadDir(backup);
    if err != nil {
        return false, NewError("打开备份目录出错: " + err.Error());
    }
    *total += len(dir);
    glib.IdleAdd(func() { progressBar.SetFraction(float64(*verified) / float64(*total)); });
    // 依次验证备份目录下子项
    for _, fi := range dir {
        // 检测取消
        muVerify.RLock();
        canceled := cancelVerify;
        muVerify.RUnlock();
        if canceled {
            return true, nil;
        }
        //开始验证
        sourceSub := path.Join(source, fi.Name());
        backupSub := path.Join(backup, fi.Name());
        if fi.IsDir() {
            if !isFile(sourceSub) && !isFolder(sourceSub) {
                // 目录 sourceSub 已删除（包括子项）
                outputVerifyResult(sourceSub, backupSub, 1, 3, bufVerifyResult, boxVerifyResult);
            }
        } else {
            if !isFile(sourceSub) && !isFolder(sourceSub) {
                // 文件 sourceSub 已删除
                if fi.Name() == "._backup.meta" {
                    continue;
                }
                outputVerifyResult(sourceSub, sourceSub, 0, 3, bufVerifyResult, boxVerifyResult);
            }
        }
        *verified++;
        glib.IdleAdd(func() { progressBar.SetFraction(float64(*verified) / float64(*total)); });
    }
    return false, nil;
}


/**
 * 验证一个文件。
 *
 * - param backup      string, 备份保存路径
 * - param source      string, 参考原始备份路径
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param compressed  bool, 是否压缩
 *
 * - return equal      bool, 文件是否相同
 * - return err        error, 比较文件过程中发生的错误
 */
func verifyFile(backup string, source string, encrypted bool, key [16]byte, compressed bool) (equal bool, err error) {
    // 打开原始文件
    src, err := os.Open(source)
    if err != nil {
        return false, err;
    }
    defer src.Close();
    // 打开备份文件
    bak, err := os.Open(backup)
    if err != nil {
        return false, err;
    }
    defer bak.Close();
    // 比较文件大小
    if !compressed {
        srcStat, err := src.Stat()
        if err != nil {
            return false, err;
        }
        bakStat, err := bak.Stat()
        if err != nil {
            return false, err;
        }
        if srcStat.Size() != bakStat.Size() {
            return false, nil;
        }
    }
    // 创建加密器
    var cipher *rc4.Cipher;
    if encrypted {
        cipher, err = rc4.NewCipher(key[:]);
        if err != nil {
            return false, err;
        }
    }
    // 比较文件内容
    srcBuff0 := []byte{};
    bakBuff0 := make([]byte, 65536);
    for {
        bakSize, bakErr := bak.Read(bakBuff0);
        if bakErr == io.EOF || bakSize < 0 {
            srcSize, srcErr := src.Read(bakBuff0);
            if srcErr == io.EOF || srcSize < 0 {
                return true, nil;
            } else if srcErr != nil {
                return false, srcErr;
            } else {
                return false, nil;
            }
        } else if bakErr != nil {
            return false, bakErr;
        }
        cmpSize := bakSize;
        bakBuff1 := make([]byte, bakSize);
        var buf []byte;
        if compressed {
            if encrypted {
                cipher.XORKeyStream(bakBuff1, bakBuff0[0:bakSize]);
                buf, err = decompressGZIP(bakBuff1);
            } else {
                buf, err = decompressGZIP(bakBuff0[0:bakSize]);
            }
            cmpSize = len(buf);
            srcBuff0 = make([]byte, cmpSize);
        } else {
            srcBuff0 = make([]byte, bakSize);
            if encrypted {
                cipher.XORKeyStream(bakBuff1, bakBuff0[0:bakSize]);
            }
        }
        srcSize, srcErr := src.Read(srcBuff0);
        if srcErr == io.EOF || srcSize != cmpSize {
            return false, nil;
        } else if srcErr != nil {
            return false, srcErr;
        }
        equal := false;
        if compressed {
            equal = bytes.Compare(srcBuff0, buf) == 0;
        } else if encrypted {
            equal = bytes.Compare(srcBuff0, bakBuff1) == 0;
        } else {
            equal = bytes.Compare(srcBuff0, bakBuff0[0:bakSize]) == 0;
        }
        if !equal {
            return false, nil;
        }
    }
    return true, nil;
}

// ---------------------------------------------------------------------------------------------------------------------

/**
 * 验证包。
 *
 * - param file        *os.File, 打包备份文件
 * - param target      string, 原始备份目录
 * - param compressed  bool, 是否压缩
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时，指定密钥
 * - param progressBar *gtk.ProgressBar, 进度条
 * - param totalFiles  int, 总文件和目录数 (用来显示进度条)
 *
 * - return canceled   bool, 验证操作是否被取消
 * - return err        error, 验证过程中发生的错误
 */
func verifyPack(file *os.File, source string, compressed bool, encrypted bool, key [16]byte,
                bufVerifyResult *gtk.TextBuffer, boxVerifyResult *gtk.TextView, progressBar *gtk.ProgressBar,
                totalFiles int) (canceled bool, paths []string, err error) {
    var currentDir string = source;
    for {
        // 检测取消
        muRestore.RLock();
        canceled := cancelRestore;
        muRestore.RUnlock();
        if canceled {
            return true, paths, nil;
        }
        // 读头
        head := make([]byte, 3);
        size, err := file.Read(head);
        if err == io.EOF || size < 0 {
            break;
        } else if size < 3 {
            return false, paths, NewError("读文件头错误");
        } else if err != nil {
            return false, paths, err;
        }
        // 处理头
        verified := 0;
        if (head[0] >> 2) == 5 {
            // 退出目录
            currentDir, _ = filepath.Split(currentDir);
            if strings.HasSuffix(currentDir, string(os.PathSeparator)) {
                currentDir = currentDir[0:len(currentDir) - 1];
            }
            verified++;
            glib.IdleAdd(func() { progressBar.SetFraction(float64(verified) / float64(totalFiles)); });
        } else {
            // 取得名称
            nameLen := int(uint((head[1] << 8) | head[2]));
            nameBuf := make([]byte, nameLen);
            size, err = file.Read(nameBuf);
            if size < nameLen {
                return false, paths, NewError("读文件名称错误");
            }
            if err != nil {
                return false, paths, err;
            }
            name := string(nameBuf);
            paths = append(paths, path.Join(currentDir, name));
            //处理路径
            switch head[0] >> 2 {
                case 0:
                    // 验证文件
                    currentFile := path.Join(currentDir, name);
                    lenBuf := make([]byte, 8);
                    size, err := file.Read(lenBuf);
                    if size < 8 {
                        return false, paths, NewError("获取文件长度错误");
                    } else if err != nil {
                        return false, paths, err;
                    }
                    contLen := uint64(binary.BigEndian.Uint64(lenBuf[0:8]));
                    if isFile(currentFile) {
                        // 文件长度
                        equal, err := verifyPackFile(currentFile, file, compressed, encrypted, key, contLen);
                        if err != nil {
                            return false, paths, err;
                        }
                        if !equal {
                            // 文件 currentFile 已修改
                            outputVerifyResult(currentFile, currentFile, 0, 1, bufVerifyResult, boxVerifyResult);
                        }
                    } else if isFolder(currentFile) {
                        // 文件 currentFile 已删除
                        file.Seek(int64(contLen), 1);
                        outputVerifyResult(currentFile, "", 0, 3, bufVerifyResult, boxVerifyResult);
                        // 目录 currentFile 已添加（不包括子项, 在 findNewPaths 添加子项)
                        outputVerifyResult(currentFile, "", 1, 2, bufVerifyResult, boxVerifyResult);
                    } else {
                        // 文件 currentFile 已删除
                        file.Seek(int64(contLen), 1);
                        outputVerifyResult(currentFile, currentFile, 0, 3, bufVerifyResult, boxVerifyResult);
                    }
                    verified++;
                    glib.IdleAdd(func() { progressBar.SetFraction(float64(verified) / float64(totalFiles)); });
                case 1:
                    // 验证目录
                    currentDir = path.Join(currentDir, name);
                    if isFile(currentDir) {
                        // 目录 currentDir 已删除（不包括子项）
                        outputVerifyResult(currentDir, currentDir, 1, 3, bufVerifyResult, boxVerifyResult);
                        // 文件 sourceSub 已添加
                        outputVerifyResult(currentDir, currentDir, 0, 2, bufVerifyResult, boxVerifyResult);
                    } else if !isFolder(currentDir) {
                        // 目录 currentDir 已删除（不包括子项）
                        outputVerifyResult(currentDir, currentDir, 1, 3, bufVerifyResult, boxVerifyResult);
                    }
                case 2: // 连接
                    break;
                case 3: // 管道
                    break;
            }
        }
    }
    return false, paths, nil;
}


/**
 * 验证包中一个文件与源文件是否相同
 *
 * - param source      string, 正在校验的文件路径
 * - param file        *os.File, 打包备份文件
 * - param compressed  bool, 是否压缩
 * - param encrypted   bool, 是否加密
 * - param key         [16]byte, 加密时, 指定密钥
 * - param length      uint64, 备份包中待验证文件的长度
 *
 * - return equal      bool, 文件是否相同
 * - return err        error, 比较文件过程中发生的错误
 */
func verifyPackFile(source string, file *os.File, compressed bool, encrypted bool, key [16]byte,
                    length uint64) (bool, error) {
    // 打开原始文件
    src, err := os.Open(source)
    if err != nil {
        return false, err;
    }
    defer src.Close();
    // 比较文件大小
    if !compressed {
        srcStat, err := src.Stat()
        if err != nil {
            return false, err;
        }
        if srcStat.Size() != int64(length) {
            file.Seek(int64(length), 1);
            return false, nil;
        }
    }
    // 创建加密器
    var cipher *rc4.Cipher;
    if encrypted {
        cipher, err = rc4.NewCipher(key[:]);
        if err != nil {
            return false, err;
        }
    }
    // 比较文件内容
    srcBuff0 := []byte{};
    bakBuff0 := []byte{};
    remainingSize := length;
    for {
        // 备份结束
        if remainingSize <= 0 {
            // 检查源是否结束
            srcBuff0 := make([]byte, 64);
            srcSize, srcErr := src.Read(srcBuff0);
            if srcErr == io.EOF || srcSize < 0 {
                return true, nil;
            } else if srcErr != nil {
                return false, srcErr;
            } else {
                return false, nil;
            }
        }
        // 读取备份文件
        readSize := 65536;
        if remainingSize < 65536 {
            readSize = int(remainingSize);
        }
        bakBuff0 = make([]byte, readSize);
        bakSize, err := file.Read(bakBuff0);
        if err == io.EOF || readSize != bakSize {
            return false, NewError("备份文件已损坏");
        } else if err != nil {
            return false, err;
        }
        remainingSize -= uint64(readSize);
        // 解密/解压备份文件
        cmpSize := bakSize;
        bakBuff1 := make([]byte, bakSize);
        var buf []byte;
        if compressed {
            if encrypted {
                cipher.XORKeyStream(bakBuff1, bakBuff0[0:bakSize]);
                buf, err = decompressGZIP(bakBuff1);
            } else {
                buf, err = decompressGZIP(bakBuff0[0:bakSize]);
            }
            cmpSize = len(buf);
            srcBuff0 = make([]byte, cmpSize);
        } else {
            if encrypted {
                cipher.XORKeyStream(bakBuff1, bakBuff0[0:bakSize]);
            }
            srcBuff0 = make([]byte, bakSize);
        }
        // 读源文件
        srcSize, srcErr := src.Read(srcBuff0);
        if srcErr == io.EOF || srcSize != cmpSize {
            return false, nil;
        } else if srcErr != nil {
            return false, srcErr;
        }
        equal := false;
        if compressed {
            equal = bytes.Compare(srcBuff0, buf) == 0;
        } else if encrypted {
            equal = bytes.Compare(srcBuff0, bakBuff1) == 0;
        } else {
            equal = bytes.Compare(srcBuff0, bakBuff0[0:bakSize]) == 0;
        }
        if !equal {
            return false, nil;
        }
    }
    return true, nil;
}


/**
 * 打包情况下，发现备份后新添加的目录和文件。
 *
 * - param source          string, 待查找的源目录
 * - param paths           []string, 备份包中包含的路径
 * - param bufVerifyResult *gtk.TextBuffer, 验证结果输出缓存
 * - param boxVerifyResult *gtk.TextView, 验证结果输出框
 *
 * - return canceled       bool, 查找操作是否被取消
 * - return err            error, 查找过程中发生的错误
 */
func findNewPaths(source string, paths []string, bufVerifyResult *gtk.TextBuffer,
                  boxVerifyResult *gtk.TextView) (bool, error) {
    // 打开源目录, 搜索新添加的文件
    dir, err := ioutil.ReadDir(source);
    if err != nil {
        return false, NewError("打开源目录出错: " + err.Error());
    }
    for _, fi := range dir {
        // 检测取消
        muVerify.RLock();
        canceled := cancelVerify;
        muVerify.RUnlock();
        if canceled {
            return true, nil;
        }
        //开始验证
        sourceSub := path.Join(source, fi.Name());
        idx := sort.SearchStrings(paths, sourceSub);
        if idx < len(paths) && paths[idx] != sourceSub {
            if isFolder(sourceSub) {
                // 文件夹 sourceSub 已添加
                outputVerifyResult(sourceSub, sourceSub, 1, 2, bufVerifyResult, boxVerifyResult);
            } else if isFile(sourceSub) {
                // 文件 sourceSub 已添加
                outputVerifyResult(sourceSub, sourceSub, 0, 2, bufVerifyResult, boxVerifyResult);
            }
        } else if isFolder(sourceSub) {
            canceled, err = findNewPaths(sourceSub, paths, bufVerifyResult, boxVerifyResult);
            if err != nil {
                return false, err;
            } else if canceled {
                return true, nil;
            }
        }
    }
    return false, nil;
}

// =====================================================================================================================

/**
 * 使用 Go 内置 gzip 压缩算法压缩数据。
 *
 * - param  data       []byte, 待压缩数据。
 *
 * - return compressed []byte, 压缩后的数据
 * - return err        error, 压缩操作发生的错误
 */
func compressGZIP(data []byte) (compressed []byte, err error) {
    var buf bytes.Buffer;
    gz := gzip.NewWriter(&buf);
    _, err = gz.Write(data);
    if err != nil {
        return;
    }
    if err = gz.Flush(); err != nil {
        return;
    }
    if err = gz.Close(); err != nil {
        return;
    }
    compressed = buf.Bytes();
    return;
}


/**
 * 使用 Go 内置 gzip 压缩算法解压缩数据。
 *
 * - param  data       []byte, 待解压缩数据。
 *
 * - return extracted  []byte, 解压缩后的数据
 * - return err        error, 解压缩操作发生的错误
 */
func decompressGZIP(data []byte) (extracted []byte, err error) {
    buf := bytes.NewBuffer(data);
    var rd io.Reader;
    rd, err = gzip.NewReader(buf);
    if err != nil {
        return;
    }
    var restored bytes.Buffer
    _, err = restored.ReadFrom(rd)
    if err != nil {
        return;
    }
    extracted = restored.Bytes();
    return;
}
