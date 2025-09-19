package routers

import (
	"github.com/xxjwxc/ginrpc"
)

func init() {
	ginrpc.SetVersion(1758268070)
	ginrpc.AddGenOne("Shares.AddGroup", "shares.add_group", []string{"post"}, []ginrpc.GenThirdParty{}, `添加一个组织`)
	ginrpc.AddGenOne("Shares.AddMyCode", "shares.add_my_code", []string{"post"}, []ginrpc.GenThirdParty{}, `给自己添加一个监听`)
	ginrpc.AddGenOne("Shares.Dayliy", "shares.dayliy", []string{"post"}, []ginrpc.GenThirdParty{}, ``)
	ginrpc.AddGenOne("Shares.DeleteMyCode", "shares.delete_my_code", []string{"post"}, []ginrpc.GenThirdParty{}, `删除一个监听`)
	ginrpc.AddGenOne("Shares.GetAllCodeName", "shares.get_all_code_name", []string{"post"}, []ginrpc.GenThirdParty{}, `获取所有中文`)
	ginrpc.AddGenOne("Shares.GetGroup", "shares.get_group", []string{"post"}, []ginrpc.GenThirdParty{}, `获取分组`)
	ginrpc.AddGenOne("Shares.GetMsg", "shares.get_msg", []string{"post"}, []ginrpc.GenThirdParty{}, `获取消息`)
	ginrpc.AddGenOne("Shares.GetMyCode", "shares.get_my_code", []string{"post"}, []ginrpc.GenThirdParty{}, ``)
	ginrpc.AddGenOne("Shares.GetMyGroup", "shares.get_my_group", []string{"post"}, []ginrpc.GenThirdParty{}, `获取我的组织`)
	ginrpc.AddGenOne("Shares.Gets", "shares.gets", []string{"post"}, []ginrpc.GenThirdParty{}, `精确查找代码`)
	ginrpc.AddGenOne("Shares.HaveNewMsg", "shares.have_new_msg", []string{"post"}, []ginrpc.GenThirdParty{}, ``)
	ginrpc.AddGenOne("Shares.Minute", "shares.minute", []string{"post"}, []ginrpc.GenThirdParty{}, `获取分时图`)
	ginrpc.AddGenOne("Shares.Search", "shares.search", []string{"post"}, []ginrpc.GenThirdParty{}, `搜索`)
	ginrpc.AddGenOne("Shares.UpsetGroupCode", "shares.upset_group_code", []string{"post"}, []ginrpc.GenThirdParty{}, ``)
	ginrpc.AddGenOne("Analy.AnalyCode", "analy.analy_code", []string{"post"}, []ginrpc.GenThirdParty{}, ``)
}
