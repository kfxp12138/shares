package analy

import (
	"fmt"
	"shares/internal/core"
	"shares/internal/model"
	"shares/internal/service/event"
	"strings"
	"time"

	"github.com/xxjwxc/public/mylog"
	"github.com/xxjwxc/public/tools"
	"gorm.io/datatypes"
)

type Msg struct {
	openid  string
	name    string
	code    string
	key     string
	price   float64
	percent float64
	desc    string
	tag     string //  标记(min,daily)
	color   string // 消息颜色
	remark  string
}

// 日线多空多头排列提醒
func maDaily() { // 日线多空多头排列提醒
	if !event.IsWorkDay() {
		return
	}

	day0 := tools.GetUtcDay0(time.Now())
	orm := core.Dao.GetDBw()
	var msgs []*Msg
	infomai, _ := model.SharesWatchTblMgr(orm.Where("ai = ? and vaild = ?", true, true)).Gets()
	var codes []string
	mp := make(map[string][]string)
	for _, v := range infomai {
		mp[v.Code] = append(mp[v.Code], v.OpenID)
		codes = append(codes, v.Code)
	}

	_mp := getMp(codes)

	for code, openids := range mp {
		list, err := model.SharesDailyTblMgr(orm.Where("day0 < ?", day0).Order("day0 desc").Limit(2)).GetFromCode(code)
		if err != nil {
			continue
		}
		if len(list) < 2 { // 不需要
			continue
		}
		sharesInfo, err := event.GetShare(code, true)
		if err != nil {
			continue
		}
		// 多头排列
		if !(list[1].Ma5 >= list[1].Ma10 && list[1].Ma10 >= list[1].Ma20) { // 上上日是非多头排列
			if list[0].Ma5 >= list[0].Ma10 && list[0].Ma10 >= list[0].Ma20 { // 上一日是多头排列(提醒)
				for _, openid := range openids {
					msgs = append(msgs, &Msg{
						openid:  openid,
						name:    fmt.Sprintf("%v(%v)", sharesInfo.Name, code),
						code:    code,
						key:     "智能提醒",
						price:   sharesInfo.Price,
						percent: sharesInfo.Percent,
						desc:    "日线多头排列",
						tag:     "daily",
						color:   getOpenIdColor(openid, 1),
						remark:  strings.Join(_mp[code], "\n"),
					})
				}
			}
		}

		// 多空排列
		if !(list[1].Ma5 < list[1].Ma10 && list[1].Ma10 < list[1].Ma20) { // 上上日是非多空排列
			if list[0].Ma5 < list[0].Ma10 && list[0].Ma10 < list[0].Ma20 { // 上一日是多空排列(提醒)
				for _, openid := range openids {
					msgs = append(msgs, &Msg{
						openid:  openid,
						name:    fmt.Sprintf("%v(%v)", sharesInfo.Name, code),
						code:    code,
						key:     "智能提醒",
						price:   sharesInfo.Price,
						percent: sharesInfo.Percent,
						desc:    "日线空头排列",
						tag:     "daily",
						color:   getOpenIdColor(openid, -1),
						remark:  strings.Join(_mp[code], "\n"),
					})
				}
			}
		}
	}

	if len(msgs) > 0 {
		sendmsg(msgs)
	}
}

func sendmsg(msgs []*Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	orm := core.Dao.GetDBw()
	list := make([]model.MsgTbl, 0, len(msgs))
	for _, v := range msgs {
		list = append(list, model.MsgTbl{
			OpenID:    v.openid,
			Code:      v.code,
			Key:       v.key,
			Desc:      v.desc,
			Price:     v.price,
			Percent:   v.percent,
			Day:       datatypes.Date(time.Now()),
			CreatedAt: time.Now(),
		})
	}
	err := model.MsgTblMgr(orm.DB).Save(&list).Error
	if err != nil {
		mylog.Error(err)
	}
	return err
}
