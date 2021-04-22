/*
作者：RecordingTheSmile
项目名称：若离SMTP发送API
项目版本：1.0
环境要求：内核版本不得过低（若提示内核版本过低则请安装新版内核或升级系统）
其他要求：使用数据库时请安装MySQL数据库，其他种类数据库请自行修改；
		若使用Qmsg推送请务必注册Qmsg API Key之后填写进配置文件;
		配置文件config.toml必须置于与可执行文件同文件夹下;
特别提醒：若需使用SqLite3数据库，在修改完之后请自行在目标系统上编译，因为GORM在使用SqLite3时对Glibc版本有依赖
*/
package main

import (
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

var Db *gorm.DB

const qmsgurl = "https://qmsg.zendee.cn/send/"

type body struct {
	To      string
	Title   string
	Content string
}
type Msglog struct {
	ID        int    `gorm:"autoIncrement;primaryKey"`
	Content   string `gorm:"type:LongText"`
	Title     string `gorm:"LongText"`
	To        string `gorm:"LongText"`
	Timestamp int64
}

func main() {
	//加载配置文件，配置文件应置于与可执行文件相同文件夹下，文件名为config.toml
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalln("错误：无法读取配置文件！错误信息：", err)
	}
	//将配置文件中的配置项读入
	viper.SetDefault("smtp.port", "25")
	smtpaddr := viper.GetString("smtp.address")
	smtpport := viper.GetString("smtp.port")
	smtpusrname := viper.GetString("smtp.username")
	smtppasswd := viper.GetString("smtp.password")
	smtpnickname := viper.GetString("smtp.nickname")
	if smtpaddr == "" || smtpusrname == "" || smtppasswd == "" || smtpnickname == "" {
		log.Fatalln("错误：SMTP配置不完整！")
	}
	auth := smtp.PlainAuth("", smtpusrname, smtppasswd, smtpaddr)
	qmsgkey := viper.GetString("qmsg.key")
	viper.SetDefault("web.port", "80")
	viper.SetDefault("web.tlsport", "443")
	webport := viper.GetString("web.port")
	webtlsport := viper.GetString("web.tlsport")
	webtlscert := viper.GetString("web.tlscert")
	webtlskey := viper.GetString("web.tlskey")
	viper.SetDefault("mysql.address", "127.0.0.1")
	mysqladdr := viper.GetString("mysql.address")
	viper.SetDefault("mysql.port", "3306")
	mysqlport := viper.GetString("mysql.port")
	mysqlusrname := viper.GetString("mysql.usrname")
	mysqlpasswd := viper.GetString("mysql.passwd")
	mysqldbname := viper.GetString("mysql.dbname")
	//初始化mysql
	if mysqlpasswd == "" || mysqlusrname == "" || mysqldbname == "" {
		log.Println("提示：MySQL信息填写不完整，将不记录推送信息！")
	} else {
		db, err := gorm.Open(mysql.Open(fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local", mysqlusrname, mysqlpasswd, mysqladdr, mysqlport, mysqldbname)), &gorm.Config{NamingStrategy: schema.NamingStrategy{SingularTable: true}})
		if err != nil {
			log.Fatalln("错误：MySQL连接失败！错误信息：", err)
		}
		Db = db
		err = db.AutoMigrate(&Msglog{})
		if err != nil {
			log.Fatalln("错误：无法初始化数据库！错误信息：", err)
		}
	}
	//初始化fiber
	app := fiber.New(fiber.Config{
		Prefork:              true,
		ServerHeader:         "Ruoli-SMTP",
		BodyLimit:            5 * 1024 * 1024,
		ReadTimeout:          60 * time.Second,
		WriteTimeout:         60 * time.Second,
		IdleTimeout:          60 * time.Second,
		CompressedFileSuffix: ".gz",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			log.Println("错误：", err)
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"success": false,
				"msg":     "服务器内部错误，请检查控制台日志！",
			})
		},
	})
	//防止大量请求
	app.Use(limiter.New(limiter.Config{
		Max: 60,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IPs()[0]
		},
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"success": false,
				"msg":     "API请求过于频繁!",
			})
		},
	}))
	//定义api路由组
	api := app.Group("/api")
	{
		//smtp推送逻辑，访问/api/mail即执行
		api.Post("/mail", func(c *fiber.Ctx) error {
			p := new(body)
			if err := c.BodyParser(p); err != nil {
				return err
			}
			if p.To == "" || p.Content == "" || p.Title == "" {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"success": false,
					"msg":     "未传入必要参数！",
				})
			}
			msg := []byte(fmt.Sprintf("To:%s\r\n"+
				"Subject:%s\r\n"+
				"\r\n"+
				"%s\r\n", p.To, p.Title, p.Content))
			if err := smtp.SendMail(smtpaddr+":"+smtpport, auth, smtpnickname, []string{p.To}, msg); err != nil {
				log.Println("发送邮件错误：", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"success": false,
					"msg":     "发送邮件失败！请查看控制台信息！",
				})
			}
			go logInDatabase(p.To, p.Content, p.Title)
			return c.Status(http.StatusOK).JSON(fiber.Map{
				"success": true,
				"msg":     "邮件发送成功！",
			})
		})
		//Qmsg推送逻辑，访问/api/qmsg即执行
		api.Post("/qmsg", func(c *fiber.Ctx) error {
			if qmsgkey == "" {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"success": false,
					"msg":     "服务器端未配置Qmsg！",
				})
			}
			p := new(body)
			if err := c.BodyParser(p); err != nil {
				return err
			}
			if p.To == "" || p.Content == "" || p.Title == "" {
				return c.Status(http.StatusBadRequest).JSON(fiber.Map{
					"success": false,
					"msg":     "未传入必要参数！",
				})
			}
			url := qmsgurl + qmsgkey
			msg := "标题：" + p.Title + "\n" + p.Content
			resp, err := http.Post(url, "text/plain", strings.NewReader(msg))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			result, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			issuccess, err := jsonparser.GetBoolean(result, "success")
			if err != nil {
				return err
			}

			if !issuccess {
				reason, err := jsonparser.GetString(result, "reason")
				if err != nil {
					return err
				}
				return c.Status(fiber.StatusOK).JSON(fiber.Map{
					"success": false,
					"msg":     reason,
				})
			} else {
				go logInDatabase(p.To, p.Content, p.Title)
				return c.Status(fiber.StatusOK).JSON(fiber.Map{
					"success": true,
					"msg":     "Qmsg推送成功！",
				})
			}
		})
	}
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).Type("html", "utf-8").Send([]byte(`<h1 alien="center">404 Not Found</h1>`))
	})
	//显示logo
	fmt.Println(`

          _____                    _____            _____                    _____                _____                    _____          
         /\    \                  /\    \          /\    \                  /\    \              /\    \                  /\    \         
        /::\    \                /::\____\        /::\    \                /::\____\            /::\    \                /::\    \        
       /::::\    \              /:::/    /       /::::\    \              /::::|   |            \:::\    \              /::::\    \       
      /::::::\    \            /:::/    /       /::::::\    \            /:::::|   |             \:::\    \            /::::::\    \      
     /:::/\:::\    \          /:::/    /       /:::/\:::\    \          /::::::|   |              \:::\    \          /:::/\:::\    \     
    /:::/__\:::\    \        /:::/    /       /:::/__\:::\    \        /:::/|::|   |               \:::\    \        /:::/__\:::\    \    
   /::::\   \:::\    \      /:::/    /        \:::\   \:::\    \      /:::/ |::|   |               /::::\    \      /::::\   \:::\    \   
  /::::::\   \:::\    \    /:::/    /       ___\:::\   \:::\    \    /:::/  |::|___|______        /::::::\    \    /::::::\   \:::\    \  
 /:::/\:::\   \:::\____\  /:::/    /       /\   \:::\   \:::\    \  /:::/   |::::::::\    \      /:::/\:::\    \  /:::/\:::\   \:::\____\ 
/:::/  \:::\   \:::|    |/:::/____/       /::\   \:::\   \:::\____\/:::/    |:::::::::\____\    /:::/  \:::\____\/:::/  \:::\   \:::|    |
\::/   |::::\  /:::|____|\:::\    \       \:::\   \:::\   \::/    /\::/    / ~~~~~/:::/    /   /:::/    \::/    /\::/    \:::\  /:::|____|
 \/____|:::::\/:::/    /  \:::\    \       \:::\   \:::\   \/____/  \/____/      /:::/    /   /:::/    / \/____/  \/_____/\:::\/:::/    / 
       |:::::::::/    /    \:::\    \       \:::\   \:::\    \                  /:::/    /   /:::/    /                    \::::::/    /  
       |::|\::::/    /      \:::\    \       \:::\   \:::\____\                /:::/    /   /:::/    /                      \::::/    /   
       |::| \::/____/        \:::\    \       \:::\  /:::/    /               /:::/    /    \::/    /                        \::/____/    
       |::|  ~|               \:::\    \       \:::\/:::/    /               /:::/    /      \/____/                          ~~          
       |::|   |                \:::\    \       \::::::/    /               /:::/    /                                                    
       \::|   |                 \:::\____\       \::::/    /               /:::/    /                                                     
        \:|   |                  \::/    /        \::/    /                \::/    /                                                      
         \|___|                   \/____/          \/____/                  \/____/                                                       


`)
	log.Println("开始监听端口")
	// 初始化监听服务
	if webtlscert == "" || webtlskey == "" {
		err := app.Listen(":" + webport)
		if err != nil {
			log.Fatalln("错误：无法监听HTTP端口！错误信息：", err)
		}
	} else {
		err := app.ListenTLS(":"+webtlsport, webtlscert, webtlskey)
		if err != nil {
			log.Fatalln("错误：无法监听HTTPS端口！错误信息：", err)
		}
	}
}
func logInDatabase(To string, Content string, Title string) {
	if Db == nil {
		return
	}
	Db.Model(&Msglog{}).Create(&Msglog{
		Content:   Content,
		Title:     Title,
		To:        To,
		Timestamp: time.Now().Unix(),
	})
}
