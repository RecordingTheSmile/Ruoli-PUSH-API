# 若离推送API
若离脚本消息推送API，支持SMTP邮件推送和Qmsg推送，支持推送记录。
# 使用方法
1.下载最新Release中的对应版本，解压  
2.根据文件中的注释修改config.toml  
3.运行rlsmtp主程序  
4.修改若离脚本中action文件夹下的rlMessage.py，将其中的
```python
api = 'http://mail.ruoli.cc/api/sendMail'
```
改为
```python
api = '你的网址+/api/mail'
```
若要使用Qmsg推送，请填写好Qmsg API key之后将上述代码改为
```python
api = '你的网址+/api/qmsg'
```
# 注意事项
1.Release中仅编译了Windows版本和Linux版本，支持的平台均为X86_64，若有其他系统和平台需求请自行下载源码编译  
2.本项目与若离本人无关，仅为个人娱乐项目
