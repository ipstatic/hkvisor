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
  "os"
  "strings"
  "sync"
  "time"

  "gopkg.in/gomail.v2"
  "gopkg.in/yaml.v2"
)

const (
  RFC3339NOZ = "2006-01-02T15:04:05"
  TZ = "America/New_York"
)

var (
	configFile = flag.String("config.file", "hkvisor.yml", "hkvisor configuration file.")
	verbose = flag.Bool("verbose", false, "verbose output")
  wg = &sync.WaitGroup{}
  events = make(chan Event)
)

type Camera struct {
  Name string `yaml:"name" json:"name"`
  IpAddress string `yaml:"ip_address" json:"ip_address"`
  Username string `yaml:"username" json:"username"`
  Password string `yaml:"password" json:"password"`
}

type Config struct {
  Cameras []Camera     `yaml:"cameras" json:"cameras"`
  Receivers Receiver `yaml:"receivers" json:"receivers"`
}

type DispatchEvent struct {
  Type string
  Attempts int
  Delivered bool
  Timestamp time.Time
  Image string
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
  Active bool
  Camera Camera
}

type Receiver struct {
  Smtp SmtpReceiver `yaml:"smtp" json:"smtp"`
}

type SmtpReceiver struct {
  From string `yaml:"from" json:"from"`
  To string `yaml:"to" json:"to"`
  Server string `yaml:"server" json:"server"`
  Port int `yaml:"port" json:"port"`
  Username string `yaml:"username" json:"username"`
  Password string `yaml:"password" json:"password"`
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

func SubscribeEvents(config Config, camera Camera) {
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
  e := Event{Active: false}
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

    xml.Unmarshal(body, &e)
    e.Camera = camera

    if *verbose { log.Printf("%s event: %s (%s - %d)", e.Camera.Name, e.Type, e.State, e.Id) }

    switch e.State {
    case "active":
      if !e.Active { events <- e}
      e.Active = true
    case "inactive":
      e.Active = false
    }
  }
}

func CameraSafeName(name string) string {
  return strings.Replace(name, " ", "_", -1)
}

func CaptureImage(camera Camera) string {
  client := &http.Client{}
  req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/Streaming/channels/1/picture", camera.IpAddress), nil)
  if err != nil{
    log.Fatal(err)
  }

  req.SetBasicAuth(camera.Username, camera.Password)

  resp, err := client.Do(req)
  if err != nil{
    log.Fatal(err)
  }

  defer resp.Body.Close()

  filename := fmt.Sprintf("/tmp/%s.jpg", CameraSafeName(camera.Name))
  file, err := os.Create(filename)
  if err != nil{
    log.Fatal(err)
  }

  _, err = io.Copy(file, resp.Body)
  if err != nil{
    log.Fatal(err)
  }
  file.Close()
  return filename
}

func DispatchEvents(config Config) {
  des := make(map[string]DispatchEvent)
  for {
    e := <-events

    des[e.Camera.Name] = DispatchEvent{
      Type: e.Type,
      Timestamp: time.Now(),
      Attempts: 0,
      Delivered: false,
      Image: CaptureImage(e.Camera),
    }

    // check if event has been notified yet. if not, attempt to send notification.
    // if failure, do not mark as being sent yet, there will be a retry.
    // have a max retry of X (associated with event/camera)
    for camera, de := range des {
      if !de.Delivered && de.Attempts < 5 {
        de.Attempts++
        log.Printf("%s event: %s", camera, de.Type)
        de.Delivered = Notify(config.Receivers.Smtp, camera, de.Type, de.Image)
        des[camera] = de
      }
    }

  }
}

func Notify(c SmtpReceiver, camera string, event string, filename string) bool {
  m := gomail.NewMessage()
  m.SetHeader("From",c.From)
  m.SetHeader("To", c.To)
  m.SetHeader("Subject", fmt.Sprintf("%s %s Event", camera, event))
  m.Embed(filename)
  m.SetBody("text/html", `<img src="cid:Front_Door.jpg" alt="My image" />`)

  d := gomail.NewPlainDialer(c.Server, c.Port, c.Username, c.Password)
  if err := d.DialAndSend(m); err != nil {
    return false
  }
  return true
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

  for _, camera := range config.Cameras {
    wg.Add(1)
    go SubscribeEvents(config, camera)
  }

  go DispatchEvents(config)
  wg.Wait()
}
