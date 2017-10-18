import { Component } from '@angular/core';
import { ApiService } from '../../services/api.service';

@Component({
  selector: 'app-root',
  templateUrl: './home.component.html',
  styleUrls: ['./home.component.css']
})
export class HomeComponent {
  private title: string = "";
  private addresses: string[];
  private newaddress: string;
  private newbtc: string[] = [];
  private used: string[] = [];
  private canBind: boolean = false;
  public count: {All: number, Free: number, Used: number};
  rows: Array< { currency: string, address: string, bind: string}> = [];
  columns = [
    { prop: 'currency' },
    { prop: 'address' },
    { prop: 'bind' },
  ];


  constructor(private api: ApiService) {

  }


  public getBTC() {
    this.api.getBTC().subscribe((data:any) => {

    let btc = JSON.parse(data._body);
      this.addresses = btc.btc_addresses;
      this.title = "all addresses:";
      this.changeRows(btc.btc_addresses)
      this.canBind = false;
    })
  }

  public getCount() {
    this.api.getCount().subscribe((data:any) => {

      let count = JSON.parse(data._body);
      this.count = count;
    })
  }

  public getFree() {
    this.api.getFreeBTC().subscribe((data:any) => {

      let btc = JSON.parse(data._body);
      this.addresses = btc.btc_addresses;
      this.title = "free addresses:";
      this.changeRows(btc.btc_addresses)
      this.canBind = true;
    })
  }

  public getUsed() {
    this.api.getUsedBTC().subscribe((data:any) => {
      let btc = JSON.parse(data._body);
      this.addresses = btc.btc_addresses;
      this.title = "used addresses:";
      this.changeRows(btc.btc_addresses)
      this.canBind = false;
    })
  }

  public sendBTC() {
    this.newbtc = [];
    this.newbtc = this.newaddress.split("\n");
    this.newbtc = this.newbtc.filter(item => item !== "");
    this.api.sendBTC(this.newbtc).subscribe((data:any)=> {
      for (let i = 0; i < this.newbtc.length; i++) {
        this.rows.push({'currency': "Bitcoin", 'address': this.newbtc[i], 'bind': this.newaddress[i]});
      }
      this.newaddress = "";
    })
  }

  public Bind(address) {
    console.log(address)
    this.used.push(address);

    this.api.setUsed(this.used).subscribe((data:any)=> {
      this.used = [];
      this.removeRow(address);
      console.log(this.rows)

    })
  }

  //help function
  public changeRows(addresses: any) {
    this.rows = [];
    for (let i=0; i<=addresses.length-1; i++) {
      this.rows.push({'currency': "Bitcoin", 'address': addresses[i], 'bind': addresses[i]})
    }
  }

  //remove from rows
  public removeRow(address: string) {
    this.rows = this.rows.filter(item => item.address !== address);
  }

}
