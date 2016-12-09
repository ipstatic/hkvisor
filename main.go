package main

import (
  "encoding/xml"
  "flag"
  "fmt"
  "io"
  "io/ioutil"
  "log"
  "mime/multipart"
  "net/http"
  "sync"
  "time"

  "gopkg.in/yaml.v2"
)

const (
  RFC3339NOZ = "2006-01-02T15:04:05"
  TZ = "America/New_York"
)

var (
	configFile = flag.String("config.file", "hkvisor.yml", "hkvisor configuration file.")
	verbose = flag.Bool("verbose", false, "verbose output")
)

type Camera struct {
  Name string `yaml:"name" json:"name"`
  IpAddress string `yaml:"ip_address" json:"ip_address"`
  Username string `yaml:"username" json:"username"`
  Password string `yaml:"password" json:"password"`
}

type Config struct {
  Cameras []Camera     `yaml:"cameras" json:"cameras"`
}

type Event struct {
  XMLName xml.Name `xml:"EventNotificationAlert"`
  IpAddress string `xml:"ipAddress"`
  Port int `xml:"portNo"`
  ChannelId int `xml:"channelID"`
  Time xmlDate `xml:"dateTime"`
  Id int `xml:"activePostCount"`
  Type string `xml:"eventType"`
  State string `xml:"eventState"`
  Description string `xml:"eventDescription"`
  Camera *Camera
  TimeZone string
}

type xmlDate struct {
  time.Time
}

func (t *xmlDate) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
  var v string
  d.DecodeElement(&v, &start)
  loc, _ := time.LoadLocation(TZ)
  last := len(v) - 5
  result, _ := time.ParseInLocation(RFC3339NOZ, v[:last], loc)
  *t = xmlDate{result}
  return nil
}

func SubscribeEvents(wg *sync.WaitGroup, config Config, camera Camera) {
  defer wg.Done()
  if *verbose { log.Printf("subscribing to camera %s", camera.Name) }

  client := &http.Client{}
  req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/ISAPI/Event/notification/alertStream", camera.IpAddress), nil)
  if err != nil{
    log.Fatal(err)
  }
  req.SetBasicAuth(camera.Username, camera.Password)
  resp, err := client.Do(req)
  if err != nil{
    log.Fatal(err)
  }
  m := multipart.NewReader(resp.Body, "boundary")
  for {
    p, err := m.NextPart()
    if err == io.EOF {
      return
    }
    if err != nil {
      log.Fatal(err)
    }
    body, err := ioutil.ReadAll(p)
    if err != nil {
      log.Fatal(err)
    }
    var e Event
    xml.Unmarshal(body, &e)
    if *verbose { log.Printf("%s event: %s (%s - %d)", camera.Name, e.Type, e.State, e.Id) }
  }
}

func init() {
  log.SetFlags(log.LstdFlags)
}

func main() {
  flag.Parse()
  yamlFile, err := ioutil.ReadFile(*configFile)
  if err != nil {
    log.Fatal(err)
  }
  var config Config
  err = yaml.Unmarshal(yamlFile, &config)
  if err != nil {
    log.Fatal(err)
  }

  wg := &sync.WaitGroup{}
  for _, camera := range config.Cameras {
    wg.Add(1)
    go SubscribeEvents(wg, config, camera)
  }
  wg.Wait()
}
