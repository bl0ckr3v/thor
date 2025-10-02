/**
 * VeChain Thor API Client
 * Utility for interacting with VeChain Thor node
 */

import axios, { AxiosInstance } from 'axios';

export interface ThorAccount {
  balance: string;
  energy: string;
  hasCode: boolean;
}

export interface ThorTransaction {
  id: string;
  chainTag: number;
  blockRef: string;
  expiration: number;
  clauses: ThorClause[];
  gasPriceCoef: number;
  gas: number;
  origin: string;
  delegator?: string;
  nonce: string;
  dependsOn?: string;
  size: number;
}

export interface ThorClause {
  to: string | null;
  value: string;
  data: string;
}

export interface ThorReceipt {
  gasUsed: number;
  gasPayer: string;
  paid: string;
  reward: string;
  reverted: boolean;
  outputs: ThorOutput[];
}

export interface ThorOutput {
  contractAddress?: string;
  events: ThorEvent[];
  transfers: ThorTransfer[];
}

export interface ThorEvent {
  address: string;
  topics: string[];
  data: string;
}

export interface ThorTransfer {
  sender: string;
  recipient: string;
  amount: string;
}

export interface ThorBlock {
  number: number;
  id: string;
  size: number;
  parentID: string;
  timestamp: number;
  gasLimit: number;
  gasUsed: number;
  totalScore: number;
  txsRoot: string;
  txsFeatures: number;
  stateRoot: string;
  receiptsRoot: string;
  signer: string;
  transactions: string[];
}

export class ThorClient {
  private client: AxiosInstance;
  private baseURL: string;

  constructor(nodeUrl: string = 'http://localhost:8669') {
    this.baseURL = nodeUrl;
    this.client = axios.create({
      baseURL: nodeUrl,
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json',
      },
    });
  }

  /**
   * Get account information
   */
  async getAccount(address: string): Promise<ThorAccount> {
    const response = await this.client.get(`/accounts/${address}`);
    return response.data;
  }

  /**
   * Get account balance
   */
  async getBalance(address: string): Promise<string> {
    const account = await this.getAccount(address);
    return account.balance;
  }

  /**
   * Get account energy (VTHO) balance
   */
  async getEnergy(address: string): Promise<string> {
    const account = await this.getAccount(address);
    return account.energy;
  }

  /**
   * Send transaction
   */
  async sendTransaction(signedTx: any): Promise<{ id: string }> {
    const response = await this.client.post('/transactions', signedTx);
    return response.data;
  }

  /**
   * Get transaction by ID
   */
  async getTransaction(txId: string): Promise<ThorTransaction> {
    const response = await this.client.get(`/transactions/${txId}`);
    return response.data;
  }

  /**
   * Get transaction receipt
   */
  async getTransactionReceipt(txId: string): Promise<ThorReceipt> {
    const response = await this.client.get(`/transactions/${txId}/receipt`);
    return response.data;
  }

  /**
   * Wait for transaction to be mined
   */
  async waitForTransaction(txId: string, timeout: number = 30000): Promise<ThorReceipt> {
    const startTime = Date.now();
    
    while (Date.now() - startTime < timeout) {
      try {
        const receipt = await this.getTransactionReceipt(txId);
        if (receipt) {
          return receipt;
        }
      } catch (error) {
        // Transaction not yet mined, continue waiting
      }
      
      // Wait 1 second before next check
      await new Promise(resolve => setTimeout(resolve, 1000));
    }
    
    throw new Error(`Transaction ${txId} not mined within ${timeout}ms`);
  }

  /**
   * Get best block
   */
  async getBestBlock(): Promise<ThorBlock> {
    const response = await this.client.get('/blocks/best');
    return response.data;
  }

  /**
   * Get block by number or ID
   */
  async getBlock(revision: string | number): Promise<ThorBlock> {
    const response = await this.client.get(`/blocks/${revision}`);
    return response.data;
  }

  /**
   * Call contract method (read-only)
   */
  async call(contractAddress: string, data: string, caller?: string): Promise<any> {
    const payload = {
      to: contractAddress,
      data: data,
      caller: caller || '0x0000000000000000000000000000000000000000',
    };
    
    const response = await this.client.post('/accounts/*', payload);
    return response.data;
  }

  /**
   * Get chain tag
   */
  async getChainTag(): Promise<number> {
    const bestBlock = await this.getBestBlock();
    return parseInt(bestBlock.id.slice(-2), 16);
  }

  /**
   * Get block ref for transaction
   */
  async getBlockRef(): Promise<string> {
    const bestBlock = await this.getBestBlock();
    return bestBlock.id.slice(0, 18);
  }

  /**
   * Check if node is ready
   */
  async isReady(): Promise<boolean> {
    try {
      await this.getBestBlock();
      return true;
    } catch (error) {
      return false;
    }
  }

  /**
   * Get node info
   */
  async getNodeInfo(): Promise<any> {
    const response = await this.client.get('/node/network/peers');
    return response.data;
  }

  /**
   * Estimate gas for transaction
   */
  async estimateGas(clauses: ThorClause[], caller?: string): Promise<number> {
    const payload = {
      clauses: clauses,
      caller: caller || '0x0000000000000000000000000000000000000000',
    };
    
    const response = await this.client.post('/accounts/*', payload);
    return response.data.gasUsed || 21000;
  }
}