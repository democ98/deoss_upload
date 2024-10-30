package main

import (
	"bufio"
	"bytes"
	"deoss_upload/adapter"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/CESSProject/cess-go-sdk/core/process"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
)

type SdkInfo struct {
	Rpc                     []string `name:"Rpc" yaml:"Rpc"`
	DeossUrl                string   `name:"DeossUrl" yaml:"DeossUrl"`
	Mnemonic                string   `name:"Mnemonic" yaml:"Mnemonic"`
	Bucket                  string   `name:"Bucket" yaml:"Bucket"`
	Territory               string   `name:"Territory" yaml:"Territory"`
	ChunksDir               string   `name:"ChunksDir" yaml:"ChunksDir"`
	FidFeedbackRecordPath   string   `name:"FidFeedbackRecordPath" yaml:"FidFeedbackRecordPath"`
	SeedMappingRecordPath   string   `name:"SeedMappingRecordPath" yaml:"SeedMappingRecordPath"`
	TorrentWs               string   `name:"TorrentWs" yaml:"TorrentWs"`
	TorrentLogin            string   `name:"TorrentLogin" yaml:"TorrentLogin"`
	TorrentSiteAccount      string   `name:"TorrentSiteAccount" yaml:"TorrentSiteAccount"`
	TorrentSitePsw          string   `name:"TorrentSitePsw" yaml:"TorrentSitePsw"`
	SeedSendForDownloadOnce int      `name:"SeedSendForDownloadOnce" yaml:"SeedSendForDownloadOnce"`
}

type TorrentRequest struct {
	Command string `json:"command"`
	Data1   []byte `json:"data1"`
	Data2   string `json:"data2"`
}
type ConReq struct {
	Command string      `json:"command"`
	Data1   interface{} `json:"data1"`
	Data2   interface{} `json:"data2"`
	Data3   interface{} `json:"data3"`
	Aop     int         `json:"aop"`
}

type TorrentResponse struct {
	Type     string `json:"type"`
	State    string `json:"state"`
	InfoHash string `json:"infohash"`
	Message  string `json:"message"`
}

type AuthRequest struct {
	Username string `json:"data1"`
	Password string `json:"data2"`
}

type AuthResponse struct {
	UserType string `json:"usertype"`
	Session  string `json:"session"`
}

func main() {
	var filepaths string
	flag.StringVar(&filepaths, "filepaths", "", "list of filepath,will upload all the files under these filepaths orderly,separate with space. Example:-filepaths \"/cess1/ /cess2/\"")

	var torrentFile string
	flag.StringVar(&torrentFile, "torrent_seed_files", "", "Send your local torrent seeds under specific file to the download site in batches.Example:-torrent_seed_files=\"/home/demoschiang/Downloads/torrent-list\"")

	var torrentDelete string
	flag.StringVar(&torrentDelete, "torrent_delete_list", "", "list of the torrent file hash that you want to delete,separate with space. Example:-torrent_seed_delete \"5eb63cc7d28adedb7bbb6c9ee04128e6ea1bb509 356227440f062eefbb7c83e1274eac01cf7f3b06\"")

	flag.Parse()

	sdkinfo, err := ParseConfig("")
	if err != nil {
		log.Fatal(err)
	}

	if filepaths != "" {
		allfilepaths := strings.Split(filepaths, " ")
		for _, file := range allfilepaths {
			sdkinfo.UploadAllFileUnderPath(file)
			log.Printf("#####################################File %s is finished##############################################\n", file)
		}
	} else if torrentFile != "" {
		sdkinfo.SendTorrentSeedForDownload(torrentFile)
	} else if torrentDelete != "" {
		torrentDeleteList := strings.Split(torrentDelete, " ")
		for _, torrentHash := range torrentDeleteList {
			sdkinfo.DeleteTorrent(torrentHash)
		}
	} else {
		log.Printf("Please check usage with --help\n")
		return
	}

}

func (config *SdkInfo) UploadAllFileUnderPath(allfilepath string) {

	uploadFileName := filepath.Base(filepath.Clean(allfilepath))
	torrentHash := filepath.Base(filepath.Dir(filepath.Clean(allfilepath)))

	//sci-hub seed file specify adapter
	recordFileName := adapter.SicHubAdapter(uploadFileName)
	log.Printf("Trying to upload %v file to deoss...", recordFileName)

	recordfile, err := os.OpenFile(filepath.Join(config.FidFeedbackRecordPath, recordFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening/creating fid record file,because:%v\n", err)
		return
	}
	defer recordfile.Close()
	writer := bufio.NewWriter(recordfile)

	files, err := os.ReadDir(allfilepath)
	if err != nil {
		log.Fatalf("Fail to read file path %s when upload all file,because:%v:", allfilepath, err)
		return
	}
	for i := 0; i < len(files); i++ {
		if !files[i].IsDir() {
			chunksDir := path.Join(config.ChunksDir, uploadFileName, files[i].Name())
			_, err = os.Stat(chunksDir)
			if err != nil {
				log.Printf("[Index%v] Trying to process file %s,and create\n", i, filepath.Join(allfilepath, files[i].Name()))
				os.MkdirAll(chunksDir, 0777)
			}

			size, num, err := process.SplitFileWithstandardSize(filepath.Join(allfilepath, files[i].Name()), chunksDir)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("The file %s has been split into %v chunks,each one size is %v", filepath.Join(allfilepath, files[i].Name()), num, size)
			res, err := process.UploadFileChunks(config.DeossUrl+"/chunks", config.Mnemonic, chunksDir, config.Territory, config.Bucket, files[i].Name(), "", num, size)
			if err != nil {
				log.Printf("Response from deoss is %s,error is :%v\n", res, err)
				i--
				continue
			}
			log.Println("upload file chunks success, response is", res)
			_, err = writer.WriteString(res + "\n")
			if err != nil {
				log.Fatalf("Error writing to record file, because:%v", err)
			}
			err = writer.Flush()
			if err != nil {
				log.Fatalf("Error flushing to file, because:%v", err)
			}
		}
	}
	config.DeleteTorrent(torrentHash)
}

func (config *SdkInfo) UploadSomeFileUnderPath(pathlist []string) {
	//todo: just upload some specify files
}

// pls build exatorrent on your machine
func (config *SdkInfo) SendTorrentSeedForDownload(allfilepath string) {
	//login first get session
	session, err := config.TorrentAuthenticate()
	if err != nil {
		log.Fatalf("Get WebSocket key from torrent site fail,because :%v", err.Error())
		return
	}

	headers := http.Header{}
	headers.Add("Cookie", "session_token="+session)
	conn, _, err := websocket.DefaultDialer.Dial(config.TorrentWs, headers)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	defer conn.Close()

	//read seed mapping record file
	mappingFile, err := os.OpenFile(filepath.Join(config.SeedMappingRecordPath), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening/creating fid record file,because:%v\n", err)
		return
	}
	defer mappingFile.Close()
	writer := bufio.NewWriter(mappingFile)

	files, err := os.ReadDir(allfilepath)
	if err != nil {
		log.Fatalf("Fail to read file path %s when upload all file,because:%v:", allfilepath, err)
		return
	}
	//range all the seed and send specific number of seed to download site
A:
	for _, file := range files {
		//wait for server processing
		time.Sleep(time.Second * 10)
		seedPath := filepath.Join(allfilepath, file.Name())
		seedByte, err := os.ReadFile(seedPath)
		if err != nil {
			continue
		}
		req := TorrentRequest{
			Command: "addtorrent",
			Data1:   seedByte,
			Data2:   "false",
		}

		reqJSON, err := json.Marshal(req)
		if err != nil {
			log.Fatal("Marshal error:", err)
		}
		err = conn.WriteMessage(websocket.TextMessage, reqJSON)
		if err != nil {
			log.Fatal("Write error:", err)
		}

		readTimeout := time.After(time.Minute)
	B:
		for {
			select {
			case <-readTimeout:
				return
			default:
				_, message, err := conn.ReadMessage()
				if err != nil {
					log.Printf("Read resp from torrent download site error:%v\n", err)
					continue B
				}
				var resp TorrentResponse
				err = json.Unmarshal(message, &resp)
				if err != nil {
					log.Fatalf("Unmarshal resp from torrent download site error:%v\n", err)
				}

				log.Printf("Received response from server :%v\n", resp)
				if resp.State == "success" && strings.Contains(resp.Message, "torrent spec added") {
					err = os.Remove(seedPath)
					if err != nil {
						log.Fatalf("remove seed file %v after send to download site fail ,because:%v\n", seedPath, err.Error())
					}
					log.Printf("Seed %s has been remove, because upload success!", seedPath)
					_, err = writer.WriteString(fmt.Sprintf("%v => %v", resp.InfoHash, file.Name()) + "\n")
					if err != nil {
						log.Fatalf("Error writing to record file, because:%v\n", err)
						return
					}
					err = writer.Flush()
					if err != nil {
						log.Fatalf("Error flushing to file, because:%v\n", err)
						return
					}
					config.SeedSendForDownloadOnce--
					if config.SeedSendForDownloadOnce <= 0 {
						log.Printf("This send seed task ,total %v files has been send!", config.SeedSendForDownloadOnce)
						return
					}
					log.Printf("**********START NEXT SEED**********")
					continue A
				}
			}
		}
	}
}

func (config *SdkInfo) DeleteTorrent(torrentHash string) {
	//login first get session
	session, err := config.TorrentAuthenticate()
	if err != nil {
		log.Fatalf("Get WebSocket key from torrent site fail,because :%v", err.Error())
		return
	}

	headers := http.Header{}
	headers.Add("Cookie", "session_token="+session)
	conn, _, err := websocket.DefaultDialer.Dial(config.TorrentWs, headers)
	if err != nil {
		log.Fatal("Dial error:", err)
	}
	defer conn.Close()

	reqRemove := ConReq{
		Command: "removetorrent",
		Data1:   torrentHash,
	}
	reqRemoveJSON, err := json.Marshal(&reqRemove)
	if err != nil {
		log.Fatalf("request for remove torrent source file fail , because :%v\n", err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, reqRemoveJSON)
	if err != nil {
		log.Fatal("Write for remove torrent file error:", err)
	}

	reqDelete := ConReq{
		Command: "deletetorrent",
		Data1:   torrentHash,
	}
	reqDeleteJSON, err := json.Marshal(&reqDelete)
	if err != nil {
		log.Fatalf("request for delete torrent source file fail , because :%v\n", err)
		return
	}

	err = conn.WriteMessage(websocket.TextMessage, reqDeleteJSON)
	if err != nil {
		log.Fatal("Write for delete torrent file error:", err)
	}

	readTimeout := time.After(time.Minute)
B:
	for {
		select {
		case <-readTimeout:
			log.Printf("Timeout when read message from server!")
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Read resp from torrent delete site error:%v\n", err)
				continue B
			}
			var resp TorrentResponse
			err = json.Unmarshal(message, &resp)
			if err != nil {
				log.Fatalf("Unmarshal resp from torrent delete site error:%v\n", err)
			}

			log.Printf("Received response from server :%v\n", resp)
			if resp.State == "success" && strings.Contains(resp.Message, "Torrent Deleted") {
				log.Printf("Delete torrent source file %v success!", resp.InfoHash)
				return
			}
		}
	}
}

func (config *SdkInfo) TorrentAuthenticate() (string, error) {
	authReq := AuthRequest{
		Username: config.TorrentSiteAccount,
		Password: config.TorrentSitePsw,
	}

	authReqJSON, err := json.Marshal(authReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth request: %v", err)
	}

	resp, err := http.Post(config.TorrentLogin, "application/json", bytes.NewBuffer(authReqJSON))
	if err != nil {
		return "", fmt.Errorf("failed to authenticate: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	var authResp AuthResponse
	err = json.Unmarshal(body, &authResp)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal auth response: %v", err)
	}

	return authResp.Session, nil
}

func ParseConfig(cpath string) (SdkInfo, error) {
	var info SdkInfo
	if cpath == "" {
		cpath = "./config.yaml"
	}
	viper.SetConfigFile(cpath)
	viper.SetConfigType("yaml")
	err := viper.ReadInConfig()
	if err != nil {
		return info, err
	}
	err = viper.Unmarshal(&info)
	if err != nil {
		return info, err
	}
	return info, nil
}
