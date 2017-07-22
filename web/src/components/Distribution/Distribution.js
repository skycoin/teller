import React from 'react';
import styled from 'styled-components';
import { rem } from 'polished';
import { Flex, Box } from 'grid-styled';

import { COLORS, SPACE } from '@skycoin/config';
import { checkStatus, getAddress } from '../../utils/distributionAPI';
import { media } from '@skycoin/utils'
import Header from '@skycoin/header';
import Footer from '@skycoin/footer';
import Text from '@skycoin/text';
import Heading from '@skycoin/heading';
import Modal, { styles } from '@skycoin/modal';
import Button from '@skycoin/button';
import Input from '@skycoin/input';
import Container from '@skycoin/container';

const Widget = styled.div`
  background-color: ${COLORS.gray[1]};
  padding: ${rem(SPACE[5])} 0;

  ${media.md.css`
    padding: ${rem(SPACE[7])} 0;
  `}
`;

export default class Distribution extends React.Component {
  constructor() {
    super();
    this.state = {
      status: '',
      skyAddress: null,
      btcAddress: '',
      addressIsOpen: false,
      statusIsOpen: false,
    };

    this.handleChange = this.handleChange.bind(this);
    this.getAddress = this.getAddress.bind(this);
    this.checkStatus = this.checkStatus.bind(this);
    this.closeModals = this.closeModals.bind(this);
  }

  getAddress() {
    if (!this.state.skyAddress) {
      return alert('Please enter a SKY address');
    }

    getAddress(this.state.skyAddress)
      .then((res) => {
        this.setState({
          addressIsOpen: true,
          btcAddress: res,
        });
      })
      .catch((err) => {
        alert(err.message);
      });
  }

  handleChange(event) {
    this.setState({
      skyAddress: event.target.value,
    });
  }

  closeModals() {
    this.setState({
      statusIsOpen: false,
      addressIsOpen: false,
    });
  }

  checkStatus() {
    if (!this.state.skyAddress) {
      return alert('Please enter a SKY address');
    }

    checkStatus(this.state.skyAddress)
      .then((res) => {
        this.setState({
          statusIsOpen: true,
          status: res,
        });
      })
      .catch((err) => {
        alert(err.message);
      });
  }

  render() {
    return (
      <div>
        <Header external />
        <Widget>
          <Modal
            contentLabel="TODO"
            style={styles}
            isOpen={this.state.addressIsOpen}
            onRequestClose={this.closeModals}
          >
            <Heading heavy color="black" fontSize={[2, 3]} my={[3, 5]}>
              BTC address for {this.state.skyAddress}
            </Heading>

            <Heading as="p" color="black" fontSize={[2, 3]} my={[3, 5]}>
              {this.state.btcAddress}
            </Heading>
          </Modal>


          <Modal
            contentLabel="TODO"
            style={styles}
            isOpen={this.state.statusIsOpen}
            onRequestClose={this.closeModals}
          >
            <Heading heavy color="black" fontSize={[2, 3]} my={[3, 5]}>
              Status for {this.state.skyAddress}
            </Heading>

            <Heading as="p" color="black" fontSize={[2, 3]} my={[3, 5]}>
              {this.state.status}
            </Heading>
          </Modal>

          <Container>
            <Flex justify="center">
              <Box width={[1 / 1, 1 / 1, 2 / 3]} py={[5, 7]}>
                <Text heavy color="black" fontSize={[2, 3]} as="div">
                  <Heading heavy as="h2" fontSize={[5, 6]} color="black" mb={[4, 6]}>
                    Skycoin distribution event
                  </Heading>

                  <p>To participate in the distribution event:</p>

                  <ul>
                    <li>Enter your Skycoin address below</li>
                    <li>You&apos;ll recieve a unique Bitcoin address to purchase SKY</li>
                    <li>Send BTC to the address—you&apos;ll recieve 500 SKY per 0.002 BTC</li>
                  </ul>

                  <p>You can check the status of your order by entering your
                    address and selecting <strong>Check status</strong>.</p>
                </Text>

                <Input
                  placeholder="Enter Skycoin address"
                  value={this.state.address}
                  onChange={this.handleChange}
                />

                <Button
                  big
                  onClick={this.getAddress}
                  color="white"
                  bg="base"
                  mr={[2, 5]}
                  fontSize={[1, 3]}
                >
                  Get address
                </Button>

                <Button
                  onClick={this.checkStatus}
                  color="base"
                  big
                  outlined
                  fontSize={[1, 3]}
                >
                  Check status
                </Button>
              </Box>
            </Flex>
          </Container>
        </Widget>
        <Footer external />
      </div>
    );
  }
}
