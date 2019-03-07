# rpcpool

### 概述 ###

实现了protorpc的连接池

## 用法 ##
```
func factory() (*rpc.Client, error) {
	return protorpc.Dial("tcp", "127.0.0.1:2234")
}
rpcPool, err := rpcpool.NewChannelPool(10, 30, factory)
if err != nil {
	panic(err)
}
rpcConn, err := rpcPool.Get()
if err != nil {
	panic(err)
}
//do something
rpcPool.PutBack(rpcConn)
```
