/* 用户信息相关（微信功能已移除） */
import Server from './def';

const user = {};

user.Clear = () => {
  Server.SessionID = '';
  Server.OpenID = '';
};

user.IsLogin = () => {
  return !!(Server.SessionID && Server.SessionID !== 'undefined');
};

user.WxLogin = async () => {
  uni.showToast({ title: '微信登录已下线，请使用开发者登录', icon: 'none' });
  return false;
};

user.getUrlCode = () => null;
user.getWeChatCode = () => {};

user.Oauth = async () => {
  return Promise.reject(new Error('weixin oauth disabled'));
};

user.GetUserInfo = async () => {
  return Promise.reject(new Error('weixin api disabled'));
};

user.UpsetUserInfo = async () => {
  return Promise.reject(new Error('weixin api disabled'));
};

user.ReLogin = async () => {
  return Promise.reject(new Error('weixin api disabled'));
};

export default user;
