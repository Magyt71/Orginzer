package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gen2brain/beeep"
	"golang.org/x/sys/windows/registry"
)

type AppConfig struct {
	WatchPaths      []string
	TargetMap       map[string]string
	DestinationPath string
	WaitTime        time.Duration
	Notifications   bool
	RunAtStartup    bool
	IsRunning       bool
	IsCleaning      bool
	mu              sync.Mutex
	Rmu             sync.RWMutex
}

type MoveRecord struct {
	FromPath string
	ToPath   string
	FileName string
	Dest     string
	Time     string
	UnixTime int64
}

type Organizer struct {
	Config          *AppConfig
	Watcher         *fsnotify.Watcher
	PrograssingFile sync.Map
	logCallback     func(string)
	RecentMoves     []MoveRecord
}

func NewOrganizer() *Organizer {
	HomDir, _ := os.UserHomeDir()

	savedConfig, err := LoadConfigFromDisk()

	var appConfig *AppConfig

	if err == nil && savedConfig != nil {
		appConfig = &AppConfig{
			WatchPaths:      savedConfig.WatchPaths,
			TargetMap:       savedConfig.TargetFolder,
			DestinationPath: savedConfig.DestinationPath,
			Notifications:   savedConfig.Notifications,
			RunAtStartup:    savedConfig.RunAtStartup,
			WaitTime:        1250 * time.Millisecond,
		}
	} else {
		appConfig = &AppConfig{
			WatchPaths: []string{filepath.Join(HomDir, "Downloads")},
			TargetMap: map[string]string{
				".jpg": "Image", ".png": "Image", ".jpeg": "Image",
				".gif": "Image", ".svg": "Image", ".webp": "Image",
				".mp4": "Video", ".mkv": "Video", ".avi": "Video",
				".pdf": "Document", ".docx": "Document", ".txt": "Document",
				".zip": "Archive", ".rar": "Archive", ".7z": "Archive",
				".exe": "Programs", ".msi": "Programs",
			},
			Notifications: true,
			RunAtStartup:  false,
			WaitTime:      1250 * time.Millisecond,
		}
	}
	var loadedMoves []MoveRecord
	if err == nil && savedConfig != nil && savedConfig.RecentMoves != nil {
		loadedMoves = savedConfig.RecentMoves
	} else {
		loadedMoves = []MoveRecord{}
	}

	org := &Organizer{
		Config:      appConfig,
		RecentMoves: loadedMoves,
	}
	// Apply the saved startup preference
	SetAutoStart(appConfig.RunAtStartup)
	return org
}

func (O *Organizer) AddToMap(ext string, folderName string) error {

	O.Config.Rmu.Lock()

	ext = strings.ToLower(strings.TrimSpace(ext))

	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	if _, exists := O.Config.TargetMap[ext]; exists {
		O.Config.Rmu.Unlock()
		return fmt.Errorf("The Ext Is Already On The Map")
	}

	O.Config.TargetMap[ext] = folderName
	O.Config.Rmu.Unlock() // Unlock before saving to avoid deadlock

	O.SaveConfigToDisk()
	return nil
}

func (O *Organizer) RemoveFromMap(ext string) {
	O.Config.Rmu.Lock()
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	delete(O.Config.TargetMap, strings.ToLower(ext))
	O.Config.Rmu.Unlock() // Unlock before saving

	O.SaveConfigToDisk()
}

func (O *Organizer) SetDestination(path string) {
	O.Config.mu.Lock()
	O.Config.DestinationPath = path
	O.Config.mu.Unlock()
	O.SaveConfigToDisk()
}

// NotificationsEnabled reads the notifications flag under the config mutex so
// the watcher goroutine never observes a torn value while the UI toggles it.
func (O *Organizer) NotificationsEnabled() bool {
	O.Config.mu.Lock()
	defer O.Config.mu.Unlock()
	return O.Config.Notifications
}

// SetNotifications updates the notifications flag under the config mutex.
func (O *Organizer) SetNotifications(enable bool) {
	O.Config.mu.Lock()
	O.Config.Notifications = enable
	O.Config.mu.Unlock()
	O.SaveConfigToDisk()
}

func (O *Organizer) AddPath(Path string) {
	O.Config.mu.Lock()
	for _, p := range O.Config.WatchPaths {
		if p == Path {
			O.Config.mu.Unlock()
			return
		}
	}

	O.Config.WatchPaths = append(O.Config.WatchPaths, Path)
	O.Config.mu.Unlock()

	O.SaveConfigToDisk()

	if O.Watcher != nil {
		O.Watcher.Add(Path)
	}
}

func (O *Organizer) log(msg string) {
	ts := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s", ts, msg)
	fmt.Println(line)
	if O.logCallback != nil {
		O.logCallback(line)
	}
}

func (O *Organizer) Start() {
	O.Config.mu.Lock()
	if O.Config.IsRunning {
		O.Config.mu.Unlock()
		return
	}

	var err error
	O.Watcher, err = fsnotify.NewWatcher()
	if err != nil {
		O.log("ERROR creating watcher: " + err.Error())
		O.Config.mu.Unlock()
		return
	}
	O.Config.IsRunning = true
	O.Config.mu.Unlock()

	go func() {
		for {
			select {
			case event, ok := <-O.Watcher.Events:
				if !ok {
					return
				}
				// نراقب إنشاء الملفات أو الكتابة النهائية فيها
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
					O.HandleFileEvent(event.Name)
				}
			case err, ok := <-O.Watcher.Errors:
				if !ok {
					return
				}
				O.log("Watcher error: " + err.Error())
			}
		}
	}()

	for _, path := range O.Config.WatchPaths {
		err = O.Watcher.Add(path)
		if err != nil {
			O.log("ERROR adding path: " + err.Error())
			continue
		}
		O.log("Watching: " + path)
	}
}

func (O *Organizer) Stop() {
	O.Config.mu.Lock()
	defer O.Config.mu.Unlock()

	if O.Watcher != nil {
		O.Watcher.Close()
		O.Watcher = nil
	}
	O.Config.IsRunning = false
	O.log("Organizer stopped.")
}

func (O *Organizer) HandleFileEvent(FilePath string) {
	// التحقق أن الملف ليس مجلداً وأنه موجود
	info, err := os.Stat(FilePath)
	if err != nil || info.IsDir() {
		return
	}

	extension := strings.ToLower(filepath.Ext(FilePath))

	O.Config.Rmu.RLock()
	TargetFolder, exists := O.Config.TargetMap[extension]
	O.Config.Rmu.RUnlock()

	if exists {
		// منع معالجة نفس الملف عدة مرات في نفس الوقت
		if _, busy := O.PrograssingFile.LoadOrStore(FilePath, true); !busy {
			go func() {
				defer O.PrograssingFile.Delete(FilePath)
				// بدلاً من الانتظار الثابت، ننتقل مباشرة للدالة التي تتحقق من جاهزية الملف
				O.ProcessMove(FilePath, TargetFolder)
			}()
		}
	}
}

func (O *Organizer) ProcessMove(FilePath string, TargetFolder string) {
	FileName := filepath.Base(FilePath)
	CurrentDir := filepath.Dir(FilePath)

	// محاولة ذكية: التحقق من جاهزية الملف كل 200 ملي ثانية لمدة أقصاها 5 ثواني
	// هذا يجعل النقل فورياً بمجرد انتهاء التحميل أو الكتابة
	for i := 0; i < 500; i++ {
		file, err := os.OpenFile(FilePath, os.O_RDWR, 0)
		if err == nil {
			file.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// إذا تم تحديد مسار وجهة موحد، ننقل الملفات إليه بدلاً من المجلد الحالي
	baseDir := CurrentDir
	O.Config.mu.Lock()
	if O.Config.DestinationPath != "" {
		baseDir = O.Config.DestinationPath
	}
	O.Config.mu.Unlock()

	destDir := filepath.Join(baseDir, TargetFolder)
	os.MkdirAll(destDir, os.ModePerm)

	finalPath := UniqPath(filepath.Join(destDir, FileName))

	if err := os.Rename(FilePath, finalPath); err != nil {
		O.log("ERROR moving " + FileName + ": " + err.Error())
		return
	}

	O.log(fmt.Sprintf("Moved: %s → %s", FileName, TargetFolder))

	if O.NotificationsEnabled() {
		beeep.Notify("File Organizer", "Moved: "+FileName+" to "+TargetFolder, "")
	}

	// تحديث قائمة السجلات بشكل آمن
	O.Config.mu.Lock()
	newLog := MoveRecord{FileName: FileName, ToPath: finalPath,
		FromPath: FilePath, Dest: TargetFolder, Time: time.Now().Format("15:04"), UnixTime: time.Now().Unix()}
	O.RecentMoves = append([]MoveRecord{newLog}, O.RecentMoves...)
	if len(O.RecentMoves) > 20 { // Modified limit to 20
		O.RecentMoves = O.RecentMoves[:20]
	}
	O.Config.mu.Unlock()

	// Save to DB immediately after a successful recorded move
	O.SaveConfigToDisk()
}

func (O *Organizer) Undolastmove() error {
	O.Config.mu.Lock()
	if len(O.RecentMoves) == 0 {
		O.Config.mu.Unlock()
		return fmt.Errorf("no file to return")
	}

	// Group moves within 2 seconds of each other
	lastBatchUnix := O.RecentMoves[0].UnixTime

	if time.Now().Unix()-lastBatchUnix > 30 {
		O.Config.mu.Unlock()
		return fmt.Errorf("cannot undo after 30 seconds")
	}

	var batch []MoveRecord
	for _, move := range O.RecentMoves {
		if lastBatchUnix-move.UnixTime <= 20 {
			batch = append(batch, move)
		} else {
			break
		}
	}
	O.Config.mu.Unlock()

	var lastErr error
	for _, move := range batch {
		// Prevent watcher from re-capturing
		O.PrograssingFile.Store(move.FromPath, true)

		if err := os.Rename(move.ToPath, move.FromPath); err != nil {
			O.log("ERROR undoing " + move.FileName + ": " + err.Error())
			lastErr = err
		} else {
			O.log("Undone: " + move.FileName)
		}

		// Keep "busy" for a bit to let events settle
		go func(path string) {
			time.Sleep(2 * time.Second)
			O.PrograssingFile.Delete(path)
		}(move.FromPath)
	}

	O.Config.mu.Lock()
	if len(batch) > 0 {
		O.RecentMoves = O.RecentMoves[len(batch):]
	}
	O.Config.mu.Unlock()

	O.SaveConfigToDisk()
	return lastErr
}

func (O *Organizer) ClearNow() error {
	O.Config.mu.Lock()
	O.Config.IsCleaning = true
	paths := make([]string, len(O.Config.WatchPaths))
	copy(paths, O.Config.WatchPaths)
	O.Config.mu.Unlock()

	defer func() {
		O.Config.mu.Lock()
		O.Config.IsCleaning = false
		O.Config.mu.Unlock()
	}()

	for _, path := range paths {
		entries, err := os.ReadDir(path)
		if err != nil {
			O.log("ERROR scanning " + path + ": " + err.Error())
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			FullPath := filepath.Join(path, entry.Name())
			ext := strings.ToLower(filepath.Ext(FullPath))
			O.Config.Rmu.RLock()
			TargetFolder, exists := O.Config.TargetMap[ext]
			O.Config.Rmu.RUnlock()

			if exists {
				if _, busy := O.PrograssingFile.LoadOrStore(FullPath, true); !busy {
					O.ProcessMove(FullPath, TargetFolder)
					O.PrograssingFile.Delete(FullPath)
				}
			}
		}
	}
	O.log("Clear Now: Finished scanning all folders.")
	return nil
}

func UniqPath(FilePath string) string {
	Dir := filepath.Dir(FilePath)
	FileName := filepath.Base(FilePath)
	Ext := filepath.Ext(FilePath)
	NameOnly := strings.TrimSuffix(FileName, Ext)

	re := regexp.MustCompile(`\(\d+\)$`)
	CleanName := re.ReplaceAllString(NameOnly, "")

	FinalPath := FilePath
	count := 1

	for {
		if _, err := os.Stat(FinalPath); os.IsNotExist(err) {
			break
		}
		NewName := fmt.Sprintf("%s(%d)%s", CleanName, count, Ext)
		FinalPath = filepath.Join(Dir, NewName)
		count++
	}
	return FinalPath
}

// SetAutoStart adds or removes the program from the Windows Registry Run key
func SetAutoStart(enable bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if enable {
		return key.SetStringValue("OrganizationApp", execPath)
	} else {
		// Ignore error if it doesn't exist when we try to delete it
		_ = key.DeleteValue("OrganizationApp")
		return nil
	}
}

// ResetWatchPaths resets the watch paths to default
func (O *Organizer) ResetWatchPaths() {
	O.Config.mu.Lock()
	HomDir, _ := os.UserHomeDir()
	O.Config.WatchPaths = []string{filepath.Join(HomDir, "Downloads")}
	O.Config.mu.Unlock()
	O.SaveConfigToDisk()
}

// ResetTargetMap resets the file extension map to default
func (O *Organizer) ResetTargetMap() {
	O.Config.Rmu.Lock()
	O.Config.TargetMap = map[string]string{
		".jpg": "Image", ".png": "Image", ".jpeg": "Image",
		".gif": "Image", ".svg": "Image", ".webp": "Image",
		".mp4": "Video", ".mkv": "Video", ".avi": "Video",
		".pdf": "Document", ".docx": "Document", ".txt": "Document",
		".zip": "Archive", ".rar": "Archive", ".7z": "Archive",
		".exe": "Programs", ".msi": "Programs",
	}
	O.Config.Rmu.Unlock()
	O.SaveConfigToDisk()
}
