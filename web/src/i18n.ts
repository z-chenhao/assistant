import chinese from '../../internal/interaction/web/locales/zh-CN.json';
const messages:Record<string,string>=chinese;
export function t(text:string):string{
 if(document.documentElement.lang!=='zh-CN')return text;
 const key=text.trim(),value=messages[key];
 return value?text.replace(key,value):text;
}
