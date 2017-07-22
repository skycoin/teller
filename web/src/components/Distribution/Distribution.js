/* eslint-disable no-alert */

import React from 'react';
import PropTypes from 'prop-types';
import styled from 'styled-components';
import Helmet from 'react-helmet';
import { Flex, Box } from 'grid-styled';
import { FormattedMessage, FormattedHTMLMessage, injectIntl } from 'react-intl';
import { rem } from 'polished';

import { COLORS, SPACE } from '@skycoin/config';
import { media } from '@skycoin/utils';
import Button from '@skycoin/button';
import Container from '@skycoin/container';
import Footer from '@skycoin/footer';
import Header from '@skycoin/header';
import Heading from '@skycoin/heading';
import Input from '@skycoin/input';
import Modal, { styles } from '@skycoin/modal';
import Text from '@skycoin/text';

import { checkStatus, getAddress } from '../../utils/distributionAPI';

const Wrapper = styled.div`
  background-color: ${COLORS.gray[1]};
  padding: ${rem(SPACE[5])} 0;

  ${media.md.css`
    padding: ${rem(SPACE[7])} 0;
  `}
`;

const Address = styled.div`
  word-break: break-all;
`;

class Distribution extends React.Component {
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
      return alert(
        this.props.intl.formatMessage({
          id: 'distribution.errors.noSkyAddress',
        }),
      );
    }

    return getAddress(this.state.skyAddress)
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
      return alert(
        this.props.intl.formatMessage({
          id: 'distribution.errors.noSkyAddress',
        }),
      );
    }

    return checkStatus(this.state.skyAddress)
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
    const { intl } = this.props;
    return (
      <div>
        <Helmet>
          <title>{intl.formatMessage({ id: 'distribution.title' })}</title>
        </Helmet>

        <Header external />

        <Wrapper>
          <Modal
            contentLabel="BTC address"
            style={styles}
            isOpen={this.state.addressIsOpen}
            onRequestClose={this.closeModals}
          >
            <Heading heavy color="black" fontSize={[2, 3]} my={[3, 5]}>
              <FormattedMessage
                id="distribution.btcAddressFor"
                values={{
                  skyAddress: this.state.skyAddress,
                }}
              />
            </Heading>

            <Heading as="p" color="black" fontSize={[2, 3]} my={[3, 5]}>
              <Address>{this.state.btcAddress}</Address>
            </Heading>
          </Modal>

          <Modal
            contentLabel="Status"
            style={styles}
            isOpen={this.state.statusIsOpen}
            onRequestClose={this.closeModals}
          >
            <Heading heavy color="black" fontSize={[2, 3]} my={[3, 5]}>
              <FormattedMessage
                id="distribution.statusFor"
                values={{
                  skyAddress: this.state.skyAddress,
                }}
              />
            </Heading>

            <Heading as="p" color="black" fontSize={[2, 3]} my={[3, 5]}>
              {this.state.status}
            </Heading>
          </Modal>

          <Container>
            <Flex justify="center">
              <Box width={[1 / 1, 1 / 1, 2 / 3]} py={[5, 7]}>
                <Heading heavy as="h2" fontSize={[5, 6]} color="black" mb={[4, 6]}>
                  <FormattedMessage id="distribution.heading" />
                </Heading>

                <Text heavy color="black" fontSize={[2, 3]} as="div">
                  <FormattedHTMLMessage id="distribution.instructions" />
                </Text>

                <Input
                  placeholder={intl.formatMessage({ id: 'distribution.enterAddress' })}
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
                  <FormattedMessage id="distribution.getAddress" />
                </Button>

                <Button
                  onClick={this.checkStatus}
                  color="base"
                  big
                  outlined
                  fontSize={[1, 3]}
                >
                  <FormattedMessage id="distribution.checkStatus" />
                </Button>
              </Box>
            </Flex>
          </Container>
        </Wrapper>

        <Footer external />
      </div>
    );
  }
}

Distribution.propTypes = {
  intl: PropTypes.shape({
    formatMessage: PropTypes.func.isRequired,
  }).isRequired,
};

export default injectIntl(Distribution);
