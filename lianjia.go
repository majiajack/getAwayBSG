package main

import (
	"context"
	"encoding/json"
	"fmt"
	"getAwayBSG/configs"
	"getAwayBSG/db"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Page struct {
	TotalPage int
	CurPage   int
}

func crawlerOneCity(cityUrl string) {
	c := colly.NewCollector()
	extensions.RandomUserAgent(c)
	extensions.Referer(c)

	c.OnHTML("#position a", func(element *colly.HTMLElement) {
		u, err := url.Parse(cityUrl)
		if err != nil {
			panic(err)
		}
		rootUrl := u.Scheme + "://" + u.Host

		goUrl := element.Attr("href")
		u, err = url.Parse(goUrl)
		if err != nil {
			fmt.Println(err)
		}
		if u.Scheme == "" {
			goUrl = rootUrl + u.Path
		} else {
			goUrl = u.String()
		}

		c.Visit(goUrl)

	})

	// 获取一页的数据
	c.OnHTML(".LOGCLICKDATA", func(e *colly.HTMLElement) {
		link := e.ChildAttr("a", "href")

		title := e.ChildText("a:first-child")
		//fmt.Println(title)

		price := e.ChildText(".totalPrice")
		price = strings.Replace(price, "万", "0000", 1)
		//fmt.Println("总价：" + price)

		unitPrice := e.ChildAttr(".unitPrice", "data-price")

		//fmt.Println("每平米：" + unitPrice)
		//fmt.Println(e.Text)
		db.Add(bson.M{"Title": title, "TotalePrice": price, "UnitPrice": unitPrice, "Link": link, "listCrawlTime": time.Now()})

	})

	c.OnHTML(".page-box", func(e *colly.HTMLElement) {
		page := Page{}
		json.Unmarshal([]byte(e.ChildAttr(".house-lst-page-box", "page-data")), &page)
		//fmt.Println(page.TotalPage)
		//fmt.Println(page.CurPage)
		if page.CurPage < page.TotalPage {
			c.Visit(cityUrl + "pg" + strconv.Itoa(page.CurPage+1) + "/")
		}

	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("列表抓取：", r.URL.String())
	})

	c.Visit(cityUrl)

}

func listCrawler() {
	confInfo := configs.Config()
	fmt.Print(confInfo)
	cityList := confInfo["cityList"].([]interface{})
	for i := 0; i < len(cityList); i++ {
		crawlerOneCity(cityList[i].(string))
	}
}

func crawlDetail() (sucnum int) {
	sucnum = 0
	c := colly.NewCollector()
	extensions.RandomUserAgent(c)
	extensions.Referer(c)
	c.OnHTML(".area .mainInfo", func(element *colly.HTMLElement) {
		db.Update(element.Request.URL.String(), bson.M{"area": strings.Replace(element.Text, "平米", "", 1), "detailCrawlTime": time.Now()})

	})

	c.OnHTML(".aroundInfo .communityName .info", func(element *colly.HTMLElement) {
		db.Update(element.Request.URL.String(), bson.M{"xiaoqu": element.Text, "detailCrawlTime": time.Now()})
	})

	c.OnHTML(".l-txt", func(element *colly.HTMLElement) {
		res := strings.Replace(element.Text, "二手房", "", 99)
		res = strings.Replace(res, " ", "", 99)
		address := strings.Split(res, ">")
		db.Update(element.Request.URL.String(), bson.M{"address": address[1 : len(address)-1], "detailCrawlTime": time.Now()})
	})

	c.OnHTML(".transaction li", func(element *colly.HTMLElement) {
		if element.ChildText("span:first-child") == "挂牌时间" {
			db.Update(element.Request.URL.String(), bson.M{"guapaitime": element.ChildText("span:last-child"), "detailCrawlTime": time.Now()})
		}
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("详情抓取：", r.URL.String())
	})

	configInfo := configs.Config()
	client, _ := mongo.NewClient(options.Client().ApplyURI(configInfo["dburl"].(string)))
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err := client.Connect(ctx)
	if err != nil {
		fmt.Print("数据库连接失败！")
		fmt.Println(err)
	}
	db := client.Database(configInfo["dbDatabase"].(string))
	lianjia := db.Collection(configInfo["dbCollection"].(string))

	cur, _ := lianjia.Find(ctx, bson.M{"detailCrawlTime": bson.M{"$exists": false}})
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var item bson.M
		if err := cur.Decode(&item); err != nil {
			fmt.Print("数据库读取失败！")
			fmt.Println(err)
		} else {
			sucnum++
			c.Visit(item["Link"].(string))
		}

	}

	return sucnum
}

func main() {
	listFlag := make(chan int)
	detailFlag := make(chan int)

	go func() {
		listCrawler()
		listFlag <- 1
	}()

	go func() {
		zeroNum := 0
		for i := 0; i < 1; i = 0 {
			if crawlDetail() == 0 {
				zeroNum++
				if zeroNum > 3 {
					break
				}
				time.Sleep(300 * time.Second)
			}
		}
		detailFlag <- 1
	}()

	<-listFlag
	<-detailFlag
}
