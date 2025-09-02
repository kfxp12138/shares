package main

import (
    "fmt"
    "net/http"
    "os"

    "shares/internal/config"
    "shares/internal/routers"

    "github.com/gin-gonic/gin"
    "github.com/gmsec/goplugins/plugin"
    "github.com/xxjwxc/public/mydoc/myswagger"
    "github.com/xxjwxc/public/mylog"
    "github.com/xxjwxc/public/server"
    "github.com/xxjwxc/public/tools"
)

// CallBack service call backe
func CallBack() {
	mylog.SetLog(mylog.GetDefaultZap())

	// swagger
    myswagger.SetHost("http://localhost:" + config.GetPort())
	myswagger.SetBasePath("shares")
	myswagger.SetSchemes(true, false)
	// -----end --
	// event.UPPPP()

	// mylog.Error(config.GetMaxCapacity())
	// reg := registry.NewDNSNamingRegistry()
	// reg := etcdv3.NewEtcdv3NamingRegistry(clientv3.Config{
	// 	Endpoints:   config.GetEtcdInfo().Addrs,
	// 	DialTimeout: time.Second * time.Duration(config.GetEtcdInfo().Timeout),
	// })
	// grpc 相关 初始化服务
	// service := micro.NewService(
	// 	micro.WithName("gmsec.srv.shares"),
	// 	// micro.WithRegisterTTL(time.Second*30),      //指定服务注册时间
	// 	micro.WithRegisterInterval(time.Second*15), //让服务在指定时间内重新注册
	// 	micro.WithRegistryNaming(reg),
	// )
	// ----------- end

    // gin restful 相关
    router := gin.Default()
    router.Use(routers.Cors())
    // Serve static visualization and docs outside API group
    router.StaticFS("/shares/echarts", http.Dir(tools.GetCurrentDirectory()+"/echarts"))
    router.StaticFS("/shares/docs", http.Dir(tools.GetCurrentDirectory()+"/docs"))
    v1 := router.Group("/shares/api/v1")
    routers.OnInitRoot(nil, v1) // 自定义初始化
	// ------ end

    plg, err := plugin.RunHTTP(plugin.WithGin(router),
        // plugin.WithMicro(service),
        plugin.WithAddr(":"+config.GetPort()))

    if err != nil {
        mylog.Error("HTTP start failed", err)
        return
    }
    mylog.Infof("HTTP listening on :%s", config.GetPort())
    plg.Wait()
    fmt.Println("done")
}

func main() {
	if config.GetIsDev() || len(os.Args) == 0 {
		CallBack()
	} else {
		server.On(config.GetServiceConfig()).Start(CallBack)
	}
}
