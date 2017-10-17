import { Injectable } from '@angular/core';
import { Headers, Http, RequestOptions, Response } from '@angular/http';

@Injectable()
export class ApiService {
  public api = 'http://127.0.0.1:8626/';

  constructor(private http: Http) {

  }

  public getBTC() {
    return this.http.get(this.api + 'getbtc')
  }

  public getUsedBTC() {
    return this.http.get(this.api + 'getusedbtc')
  }

  public getFreeBTC() {
    return this.http.get(this.api + 'allfreebtc')
  }

  public getCount() {
    return this.http.get(this.api + 'getcountbtc')
  }

  public sendBTC(addresses: string[]) {
    const url = this.api + 'addbtc';

    const data = {
      btc_addresses: addresses,
    };

    return this.http.post(url, data,  new RequestOptions({
      headers: new Headers({'Content-Type': 'application/json' })  }));
  }

  public setUsed(addresses: string[]) {
    const url = this.api + 'setusedbtc';

    const data = {
      btc_addresses: addresses,
    };

    return this.http.post(url, data,  new RequestOptions({
      headers: new Headers({'Content-Type': 'application/json' })  }));
  }

}
